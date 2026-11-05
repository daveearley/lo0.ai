// Package migrate plans and applies a move to a local model and harness.
// A plan is built without side effects and shown to the user first; only
// Apply changes anything, and only through the Effects it is given.
//
// A plan combines two halves that vary independently: a backend prepares the
// model in a local runtime (internal/backend), and a harness points a coding
// tool at it (internal/harness/...). Adding a runtime or a harness means
// adding one implementation of the matching interface below.
package migrate

import (
	"context"
	"time"

	"lo0.ai/lo0/internal/bench"
	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/claudecode"
	"lo0.ai/lo0/internal/claudecode/setup"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/runtimes"
)

// Protocol is an API a runtime serves and a harness speaks.
type Protocol string

// Protocols.
const (
	// Anthropic is the Messages API at /v1/messages.
	Anthropic Protocol = "anthropic"
	// OpenAI is the Chat Completions API at /v1/chat/completions.
	OpenAI Protocol = "openai"
)

// Backend prepares a model in one local runtime.
type Backend interface {
	ID() runtimes.ID
	// Prepare returns how the model will be served and the steps that get
	// it there. It does no I/O.
	Prepare(req Request, protocol Protocol) (Served, []Step, error)
	// Revert undoes a runtime change one of its steps recorded. A change
	// that is already gone is not an error.
	Revert(ctx context.Context, fx Effects, c Change) error
}

// Harness points one coding tool at a served model.
type Harness interface {
	ID() harness.ID
	Name() string
	// Protocol is the API the harness speaks.
	Protocol() Protocol
	// Plan returns the steps that set the harness up and what the user does
	// afterwards. It does no I/O beyond req.LookPath.
	Plan(req Request, served Served) (Setup, error)
}

// Request is what backends and harnesses plan from.
type Request struct {
	GOOS string
	Home string
	// Getenv reads the environment lo0 was started with.
	Getenv func(string) string `json:"-"`
	// LookPath finds a program on PATH, as exec.LookPath does.
	LookPath func(string) (string, error) `json:"-"`
	Layout   claudecode.Layout
	Setup    setup.Inventory
	// Features are the target harness's feature mapping for Setup.
	Features []harness.Feature
	Model    catalog.Model
	Quant    catalog.Quant
	// Context is the context window to run the model with, in tokens.
	Context int
	// Runtime is the runtime as the scan found it.
	Runtime runtimes.Runtime
	// Installed is the runtime's name for a build of Model at Quant that is
	// already there, if any.
	Installed string
	// Target is the target harness as it is on this machine. A zero value
	// means it is not installed, so it is planned as the release its
	// installer would install.
	Target harness.Installation
	// Background is a second model to prepare alongside Model for the
	// harness's background work. Nil means Model does that work too.
	Background *Background
}

// Background is a second, smaller model a backend prepares alongside the
// main one, for work such as session titles, summaries and quick subagents.
type Background struct {
	Model catalog.Model
	Quant catalog.Quant
	// Context is the context window to run it with, in tokens.
	Context int
}

// BackgroundUser is implemented by a harness that can send its background
// work to a second model. Plans prepare one only for a harness that says it
// uses it.
type BackgroundUser interface {
	UsesBackground() bool
}

// Served is how a harness reaches the model once the backend's steps have
// run.
type Served struct {
	Runtime     runtimes.ID `json:"runtime"`
	RuntimeName string      `json:"runtimeName"`
	// BaseURL is the API root, such as http://127.0.0.1:11434. The Anthropic
	// API is at BaseURL/v1/messages and the OpenAI API at BaseURL/v1.
	BaseURL string `json:"baseUrl"`
	// Model is the name the API serves the model under.
	Model string `json:"model"`
	// DisplayName is the model's catalogue name.
	DisplayName string `json:"displayName"`
	Context     int    `json:"context"`
	// Token is the placeholder credential the runtime expects. Local
	// runtimes do not check it, but clients refuse to start without one.
	Token string `json:"token"`
	// Background is the second model, served at BaseURL alongside Model,
	// when the request asked for one and the runtime serves it.
	Background *ServedModel `json:"background,omitzero"`
	// BackgroundNote says why the runtime does not serve the background
	// model the request asked for. Empty otherwise.
	BackgroundNote string `json:"backgroundNote,omitzero"`
}

// ServedModel is one more model a runtime serves at the same address.
type ServedModel struct {
	// Model is the name the API serves the model under.
	Model string `json:"model"`
	// DisplayName is the model's catalogue name.
	DisplayName string `json:"displayName"`
	Quant       string `json:"quant"`
	Context     int    `json:"context"`
}

// BackgroundModel is the name background work is sent to: the background
// model when one is served, else the main model.
func (s Served) BackgroundModel() string {
	if s.Background != nil {
		return s.Background.Model
	}
	return s.Model
}

// checks are the requests that prove the harness reaches every model it
// will use.
func (s Served) checks(p Protocol) []Step {
	out := []Step{Check(s, p)}
	if s.Background != nil {
		bg := s
		bg.Model, bg.Context, bg.Background = s.Background.Model, s.Background.Context, nil
		out = append(out, Check(bg, p))
	}
	return out
}

// OutputTokens is how many tokens a harness should let one response use.
// Harnesses reserve this much of the window for the reply, so on a small
// local window the usual 32k would leave too little room for the
// conversation.
func (s Served) OutputTokens() int {
	return max(min(32_000, s.Context/4), 4096)
}

// Setup is a harness's half of a plan.
type Setup struct {
	Steps []Step
	// Then says how to start working once the plan has run.
	Then []string
}

// Kind classifies a step.
type Kind string

// Step kinds.
const (
	// KindManual is something the user does; lo0 explains how.
	KindManual Kind = "manual"
	// KindDownload fetches model weights through the runtime.
	KindDownload Kind = "download"
	// KindRuntime changes the runtime, such as creating a model.
	KindRuntime Kind = "runtime"
	// KindWrite writes a file.
	KindWrite Kind = "write"
	// KindCheck sends a request to the served model.
	KindCheck Kind = "check"
	// KindInstall runs a program's own installer.
	KindInstall Kind = "install"
	// KindMeasure measures a model's speed, or keeps what was measured.
	KindMeasure Kind = "measure"
)

// Step is one action in a plan.
type Step struct {
	Kind  Kind   `json:"kind"`
	Title string `json:"title"`
	// Detail is what the step will do in specifics: a command, a path, the
	// keys it sets. It never contains secret values.
	Detail []string `json:"detail,omitzero"`
	// Done is true when the step was already satisfied when planned.
	Done bool `json:"done,omitzero"`
	// Bytes is the size of a download.
	Bytes int64 `json:"bytes,omitzero"`
	// Run performs the step. For a manual step it is optional and only
	// checks whether the user has done it, failing if not.
	Run Action `json:"-"`
}

// Action performs a step. progress, which is never nil, reports work done
// out of total, in bytes for downloads.
type Action func(ctx context.Context, fx Effects, progress func(done, total int64)) error

// Plan is the whole move, ready to show and apply.
type Plan struct {
	// Command names what the plan does as lo0 commands do: "migrate",
	// "try", "remove", "pull", "install" or "benchmark".
	Command string `json:"command,omitzero"`
	// Undo is true for a plan from PlanUndo, which reverses earlier moves
	// rather than making one.
	Undo        bool        `json:"undo,omitzero"`
	Harness     harness.ID  `json:"harness"`
	HarnessName string      `json:"harnessName"`
	Runtime     runtimes.ID `json:"runtime"`
	RuntimeName string      `json:"runtimeName"`
	Model       string      `json:"model"`
	Quant       string      `json:"quant"`
	Context     int         `json:"context"`
	Served      Served      `json:"served"`
	Steps       []Step      `json:"steps"`
	Then        []string    `json:"then,omitzero"`
	// ByHand are features of the setup that lo0 does not convert, with how
	// to convert them.
	ByHand []harness.Feature `json:"byHand,omitzero"`
	// Session is set on a plan from BuildTry: the harness to start once the
	// steps have run.
	Session *Session `json:"session,omitzero"`
	// Measured is set on a plan from PlanBenchmark and filled in as it
	// runs, so the outcome can show what was measured.
	Measured *bench.Result `json:"measured,omitzero"`
}

// Pending reports whether any step still has work to do. The checks that
// close a plan only confirm what the other steps set up, so a plan whose
// other steps are all done is not pending.
func (p Plan) Pending() bool {
	for _, s := range p.Steps {
		if !s.Done && s.Kind != KindCheck {
			return true
		}
	}
	return false
}

// Build combines a backend and a harness into a plan for req, ending with a
// request to the served model that proves the two connect.
func Build(req Request, b Backend, h Harness) (Plan, error) {
	served, steps, err := b.Prepare(req, h.Protocol())
	if err != nil {
		return Plan{}, err
	}
	set, err := h.Plan(req, served)
	if err != nil {
		return Plan{}, err
	}
	p := Plan{
		Command: "migrate", Harness: h.ID(), HarnessName: h.Name(),
		Runtime: served.Runtime, RuntimeName: served.RuntimeName,
		Model: req.Model.Name, Quant: req.Quant.Name, Context: req.Context,
		Served: served,
		Steps:  append(append(steps, set.Steps...), served.checks(h.Protocol())...),
		Then:   set.Then,
	}
	for _, f := range req.Features {
		if f.Status != harness.StatusNative && f.Status != harness.StatusLost && !f.Migrated {
			p.ByHand = append(p.ByHand, f)
		}
	}
	return p, nil
}

// checkTimeout bounds the final check. The first request makes the runtime
// load the model into memory, which takes a while for large models.
const checkTimeout = 5 * time.Minute
