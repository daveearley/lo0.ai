// Package scan runs lo0's read-only stages concurrently and collects their
// results. Nothing in a scan writes to disk or changes the user's setup.
package scan

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"lo0.ai/lo0/internal/bench"

	"lo0.ai/lo0/internal/claudecode"
	"lo0.ai/lo0/internal/claudecode/install"
	"lo0.ai/lo0/internal/claudecode/setup"
	"lo0.ai/lo0/internal/codex"
	codexsetup "lo0.ai/lo0/internal/codex/setup"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/machine"
	"lo0.ai/lo0/internal/runtimes"
	"lo0.ai/lo0/internal/usage"
)

// Stage names one part of a scan.
type Stage string

// Scan stages, in display order.
const (
	StageMachine  Stage = "machine"
	StageRuntimes Stage = "runtimes"
	StageSetup    Stage = "setup"
	StageHistory  Stage = "history"
)

// Stages lists every stage in display order.
var Stages = []Stage{StageMachine, StageRuntimes, StageSetup, StageHistory}

// Status is a stage's lifecycle state.
type Status int

// Stage statuses.
const (
	StatusPending Status = iota
	StatusRunning
	StatusDone
)

// Event reports progress on one stage.
type Event struct {
	Stage  Stage
	Status Status
	// Done and Total count units of work, when the stage can measure them.
	Done, Total int
	// Warnings is how many problems the stage hit. It is set when Status is
	// StatusDone.
	Warnings int
	// Found is what the stage found, set when Status is StatusDone, so a
	// live view can show it before the whole scan is in. Only the field for
	// Stage is filled.
	Found Found
}

// Found is what one stage of a scan found.
type Found struct {
	Machine  machine.Profile
	Runtimes []runtimes.Runtime
	Setup    setup.Inventory
	Codex    codexsetup.Inventory
	Usage    usage.Profile
}

// Observer receives events. The scanner calls it from one goroutine at a
// time, so it needs no locking of its own, but it must return quickly.
type Observer func(Event)

// Warning is a problem a stage worked around.
type Warning struct {
	Stage   Stage  `json:"stage"`
	Message string `json:"message"`
}

// Result is the outcome of a scan.
type Result struct {
	GeneratedAt time.Time          `json:"generatedAt"`
	DurationMS  int64              `json:"durationMs"`
	Layout      claudecode.Layout  `json:"layout"`
	Machine     machine.Profile    `json:"machine"`
	Runtimes    []runtimes.Runtime `json:"runtimes"`
	Setup       setup.Inventory    `json:"setup"`
	// Codex is the Codex setup, when a Codex scanner was given.
	Codex codexsetup.Inventory `json:"codex,omitzero"`
	// Claude is the Claude Code builds installed and the newest release.
	Claude   install.Installs `json:"claude"`
	Usage    usage.Profile    `json:"usage"`
	Warnings []Warning        `json:"warnings,omitzero"`
	// Benchmarks are lo0's own measurements of models on this machine. The
	// scan does not read them; the application adds them from lo0's state.
	Benchmarks []bench.Result `json:"benchmarks,omitzero"`
	// Harnesses are the target harnesses as they are on this machine, by
	// ID. The scan does not read them; the application asks each harness.
	Harnesses map[harness.ID]harness.Installation `json:"harnesses,omitzero"`
}

// MachineProber profiles the hardware.
type MachineProber interface {
	Probe(ctx context.Context) (machine.Profile, error)
}

// RuntimeDetector detects one local model runtime.
type RuntimeDetector interface {
	Detect(ctx context.Context) (runtimes.Runtime, error)
}

// SetupScanner inventories the Claude Code setup.
type SetupScanner interface {
	Scan(ctx context.Context) (setup.Inventory, error)
}

// CodexScanner inventories the Codex setup.
type CodexScanner interface {
	Scan(ctx context.Context) (codexsetup.Inventory, error)
}

// ClaudeDetector finds the installed Claude Code versions.
type ClaudeDetector interface {
	Detect(ctx context.Context) (install.Installs, error)
}

// HistoryAnalyzer profiles the session history of every harness lo0 reads.
type HistoryAnalyzer interface {
	Analyze(ctx context.Context, progress usage.Progress) (usage.Profile, error)
}

// Sources are the dependencies of a Scanner.
type Sources struct {
	Layout   claudecode.Layout
	Machine  MachineProber
	Runtimes []RuntimeDetector
	Setup    SetupScanner
	// Codex is optional; without it no Codex setup is read.
	Codex CodexScanner
	// Claude is optional; without it versions are read from history only.
	Claude  ClaudeDetector
	History HistoryAnalyzer
	// Now returns the current time. It defaults to time.Now.
	Now func() time.Time
}

// Scanner runs a scan.
type Scanner struct {
	src Sources
}

// New returns a Scanner. Machine, Setup and History are required; Now
// defaults to time.Now.
func New(src Sources) (*Scanner, error) {
	if src.Machine == nil || src.Setup == nil || src.History == nil {
		return nil, errors.New("scan: Machine, Setup and History sources are required")
	}
	if src.Now == nil {
		src.Now = time.Now
	}
	return &Scanner{src: src}, nil
}

// Run executes every stage concurrently. Stage failures become warnings on
// the result; Run itself fails only if ctx is cancelled.
func (s *Scanner) Run(ctx context.Context, observe Observer) (Result, error) {
	start := s.src.Now()
	res := Result{GeneratedAt: start, Layout: s.src.Layout}
	r := &run{ctx: ctx, observe: observe}

	var wg sync.WaitGroup
	// Each stage writes only its own part of res and reports it once
	// written, so the observer never reads what another stage is writing.
	wg.Go(func() {
		r.stage(StageMachine, func() (f Found, err error) {
			res.Machine, err = s.src.Machine.Probe(ctx)
			return Found{Machine: res.Machine}, err
		})
	})
	wg.Go(func() {
		r.stage(StageRuntimes, func() (f Found, err error) {
			res.Runtimes, err = s.detectRuntimes(ctx)
			return Found{Runtimes: res.Runtimes}, err
		})
	})
	wg.Go(func() {
		r.stage(StageSetup, func() (Found, error) {
			var (
				claudeErr, codexErr error
				others              sync.WaitGroup
			)
			if s.src.Claude != nil {
				others.Go(func() { res.Claude, claudeErr = s.src.Claude.Detect(ctx) })
			}
			if s.src.Codex != nil {
				others.Go(func() { res.Codex, codexErr = s.src.Codex.Scan(ctx) })
			}
			var err error
			res.Setup, err = s.src.Setup.Scan(ctx)
			others.Wait()
			return Found{Setup: res.Setup, Codex: res.Codex}, errors.Join(err, claudeErr, codexErr)
		})
	})
	wg.Go(func() {
		r.stage(StageHistory, func() (f Found, err error) {
			res.Usage, err = s.src.History.Analyze(ctx, func(done, total int) {
				r.emit(Event{Stage: StageHistory, Status: StatusRunning, Done: done, Total: total})
			})
			return Found{Usage: res.Usage}, err
		})
	})
	wg.Wait()

	for _, note := range install.Check(res.Claude, res.Usage.Of(claudecode.Harness).Format) {
		r.warnings = append(r.warnings, Warning{Stage: StageSetup, Message: note})
	}
	for _, note := range codex.Check(res.Usage.Of(codex.Harness).Format) {
		r.warnings = append(r.warnings, Warning{Stage: StageHistory, Message: note})
	}
	res.DurationMS = s.src.Now().Sub(start).Milliseconds()
	res.Warnings = r.sortedWarnings()
	return res, ctx.Err()
}

func (s *Scanner) detectRuntimes(ctx context.Context) ([]runtimes.Runtime, error) {
	found := make([]runtimes.Runtime, len(s.src.Runtimes))
	errs := make([]error, len(s.src.Runtimes))
	var wg sync.WaitGroup
	for i, d := range s.src.Runtimes {
		wg.Go(func() { found[i], errs[i] = d.Detect(ctx) })
	}
	wg.Wait()
	return found, errors.Join(errs...)
}

type run struct {
	ctx     context.Context
	observe Observer

	mu       sync.Mutex
	warnings []Warning
}

func (r *run) emit(e Event) {
	if r.observe == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.observe(e)
}

func (r *run) stage(stage Stage, do func() (Found, error)) {
	r.emit(Event{Stage: stage, Status: StatusRunning})
	found, err := do()
	problems := flatten(r.ctx, err)
	r.mu.Lock()
	for _, p := range problems {
		r.warnings = append(r.warnings, Warning{Stage: stage, Message: p.Error()})
	}
	r.mu.Unlock()
	r.emit(Event{Stage: stage, Status: StatusDone, Warnings: len(problems), Found: found})
}

func (r *run) sortedWarnings() []Warning {
	r.mu.Lock()
	defer r.mu.Unlock()
	slices.SortStableFunc(r.warnings, func(a, b Warning) int {
		return slices.Index(Stages, a.Stage) - slices.Index(Stages, b.Stage)
	})
	return r.warnings
}

// flatten expands errors.Join trees into their leaves, dropping
// cancellation of the scan, which Run reports on its own. A probe's own
// timeout matches context.DeadlineExceeded too, but is a warning.
func flatten(ctx context.Context, err error) []error {
	if err == nil || ctx.Err() != nil && errors.Is(err, ctx.Err()) {
		return nil
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var out []error
		for _, e := range joined.Unwrap() {
			out = append(out, flatten(ctx, e)...)
		}
		return out
	}
	return []error{err}
}
