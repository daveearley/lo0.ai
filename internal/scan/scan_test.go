package scan_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"lo0.ai/lo0/internal/claudecode"
	"lo0.ai/lo0/internal/claudecode/install"
	"lo0.ai/lo0/internal/claudecode/setup"
	"lo0.ai/lo0/internal/machine"
	"lo0.ai/lo0/internal/runtimes"
	"lo0.ai/lo0/internal/scan"
	"lo0.ai/lo0/internal/usage"
)

type machineFunc func(context.Context) (machine.Profile, error)

func (f machineFunc) Probe(ctx context.Context) (machine.Profile, error) { return f(ctx) }

type runtimeFunc func(context.Context) (runtimes.Runtime, error)

func (f runtimeFunc) Detect(ctx context.Context) (runtimes.Runtime, error) { return f(ctx) }

type setupFunc func(context.Context) (setup.Inventory, error)

func (f setupFunc) Scan(ctx context.Context) (setup.Inventory, error) { return f(ctx) }

type historyFunc func(context.Context, usage.Progress) (usage.Profile, error)

func (f historyFunc) Analyze(ctx context.Context, p usage.Progress) (usage.Profile, error) {
	return f(ctx, p)
}

func sources() scan.Sources {
	clock := time.Date(2026, 11, 4, 18, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	return scan.Sources{
		Machine: machineFunc(func(context.Context) (machine.Profile, error) {
			return machine.Profile{CPU: machine.CPU{Model: "M5"}}, nil
		}),
		Runtimes: []scan.RuntimeDetector{
			runtimeFunc(func(context.Context) (runtimes.Runtime, error) { return runtimes.Runtime{ID: runtimes.Ollama}, nil }),
			runtimeFunc(func(context.Context) (runtimes.Runtime, error) { return runtimes.Runtime{ID: runtimes.LlamaCpp}, nil }),
		},
		Setup: setupFunc(func(context.Context) (setup.Inventory, error) {
			return setup.Inventory{Projects: []string{"/p"}}, nil
		}),
		History: historyFunc(func(_ context.Context, progress usage.Progress) (usage.Profile, error) {
			for i := range 3 {
				progress(i+1, 3)
			}
			return usage.Profile{Sessions: 3}, nil
		}),
		Now: func() time.Time {
			mu.Lock()
			defer mu.Unlock()
			clock = clock.Add(250 * time.Millisecond)
			return clock
		},
	}
}

func TestRunCollectsEveryStage(t *testing.T) {
	s, err := scan.New(sources())
	if err != nil {
		t.Fatal(err)
	}
	var events []scan.Event
	res, err := s.Run(t.Context(), func(e scan.Event) { events = append(events, e) })
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Machine.CPU.Model != "M5" || len(res.Setup.Projects) != 1 || res.Usage.Sessions != 3 {
		t.Errorf("result = %+v", res)
	}
	ids := []runtimes.ID{res.Runtimes[0].ID, res.Runtimes[1].ID}
	if !slices.Equal(ids, []runtimes.ID{runtimes.Ollama, runtimes.LlamaCpp}) {
		t.Errorf("runtimes = %v, want detector order kept", ids)
	}
	if res.DurationMS != 250 || len(res.Warnings) != 0 {
		t.Errorf("duration %dms, warnings %v", res.DurationMS, res.Warnings)
	}

	for _, stage := range scan.Stages {
		var statuses []scan.Status
		for _, e := range events {
			if e.Stage == stage && e.Done == 0 {
				statuses = append(statuses, e.Status)
			}
		}
		if len(statuses) < 2 || statuses[0] != scan.StatusRunning || statuses[len(statuses)-1] != scan.StatusDone {
			t.Errorf("%s statuses = %v, want running then done", stage, statuses)
		}
	}
	var progress []int
	for _, e := range events {
		if e.Stage == scan.StageHistory && e.Done > 0 {
			progress = append(progress, e.Done)
		}
	}
	if !slices.Equal(progress, []int{1, 2, 3}) {
		t.Errorf("history progress = %v", progress)
	}
}

func TestRunTurnsStageErrorsIntoWarnings(t *testing.T) {
	src := sources()
	src.Machine = machineFunc(func(context.Context) (machine.Profile, error) {
		return machine.Profile{CPU: machine.CPU{Model: "partial"}}, errors.Join(errors.New("gpu: nvidia-smi failed"), errors.New("disk: denied"))
	})
	src.Setup = setupFunc(func(context.Context) (setup.Inventory, error) {
		return setup.Inventory{}, fmt.Errorf("reading .mcp.json: %w", errors.New("bad json"))
	})
	s, err := scan.New(src)
	if err != nil {
		t.Fatal(err)
	}
	doneWarnings := map[scan.Stage]int{}
	res, err := s.Run(t.Context(), func(e scan.Event) {
		if e.Status == scan.StatusDone {
			doneWarnings[e.Stage] = e.Warnings
		}
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := []scan.Warning{
		{Stage: scan.StageMachine, Message: "gpu: nvidia-smi failed"},
		{Stage: scan.StageMachine, Message: "disk: denied"},
		{Stage: scan.StageSetup, Message: "reading .mcp.json: bad json"},
	}
	if !slices.Equal(res.Warnings, want) {
		t.Errorf("Warnings = %v, want %v", res.Warnings, want)
	}
	if res.Machine.CPU.Model != "partial" {
		t.Error("partial machine profile was dropped")
	}
	if doneWarnings[scan.StageMachine] != 2 || doneWarnings[scan.StageSetup] != 1 {
		t.Errorf("done-event warning counts = %v", doneWarnings)
	}
}

func TestRunReportsCancellation(t *testing.T) {
	src := sources()
	src.History = historyFunc(func(ctx context.Context, _ usage.Progress) (usage.Profile, error) {
		<-ctx.Done()
		return usage.Profile{}, ctx.Err()
	})
	s, err := scan.New(src)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	res, err := s.Run(ctx, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled", err)
	}
	if len(res.Warnings) != 0 {
		t.Errorf("cancellation reported as warnings: %v", res.Warnings)
	}
}

func TestNewRequiresSources(t *testing.T) {
	if _, err := scan.New(scan.Sources{}); err == nil {
		t.Fatal("New accepted empty sources")
	}
}

type claudeFunc func(context.Context) (install.Installs, error)

func (f claudeFunc) Detect(ctx context.Context) (install.Installs, error) { return f(ctx) }

func TestRunDetectsClaudeCodeAndChecksItsVersion(t *testing.T) {
	src := sources()
	src.Claude = claudeFunc(func(context.Context) (install.Installs, error) {
		return install.Installs{App: "99.0.0", Latest: "99.0.1"}, errors.New("checking the newest Claude Code release: offline")
	})
	s, err := scan.New(src)
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.Run(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Claude.App != "99.0.0" {
		t.Errorf("Claude = %+v", res.Claude)
	}
	var messages []string
	for _, w := range res.Warnings {
		if w.Stage != scan.StageSetup {
			t.Errorf("warning %q is on stage %s, want setup", w.Message, w.Stage)
		}
		messages = append(messages, w.Message)
	}
	if len(messages) != 2 || !strings.Contains(messages[0], "offline") || !strings.Contains(messages[1], "newer than the "+claudecode.VerifiedVersion) {
		t.Errorf("warnings = %q, want the lookup failure and the version note", messages)
	}
}
