package tui

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/runtimes"
)

var tryClock = time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)

// fakeTrier plans a fixed try and reports a fixed session.
type fakeTrier struct {
	fail    error
	planned *[]app.MigrationChoice
	// kept gives Claude Code a claude-local launcher for the best fit.
	kept bool
}

func (f fakeTrier) plan(c app.MigrationChoice, session bool) migrate.Plan {
	*f.planned = append(*f.planned, c)
	served := migrate.Served{
		Runtime: runtimes.Ollama, RuntimeName: "Ollama", BaseURL: "http://127.0.0.1:11434", Model: "lo0/coder-30b-a3b:256k", DisplayName: "Coder-30B-A3B", Context: 262_144,
		Background: &migrate.ServedModel{Model: "lo0/small-2b:256k", DisplayName: "Small-2B", Quant: "Q8_0", Context: 262_144},
	}
	p := migrate.Plan{
		Harness: c.Harness, HarnessName: "Claude Code on a local model", Runtime: runtimes.Ollama, RuntimeName: "Ollama",
		Model: "Coder-30B-A3B", Quant: "Q4_K_M", Context: 262_144, Served: served,
		Steps: []migrate.Step{
			{Kind: migrate.KindDownload, Title: "Coder-30B-A3B at Q4_K_M is already in Ollama", Done: true},
			{Kind: migrate.KindRuntime, Title: "Create lo0/coder-30b-a3b:256k with a 256k context window", Detail: []string{"num_ctx 262,144"}},
			{Kind: migrate.KindDownload, Title: "Download Small-2B at Q8_0", Detail: []string{"ollama pull hf.co/someone/Small-2B-GGUF:Q8_0"}, Bytes: 2_416_199_188},
			{Kind: migrate.KindRuntime, Title: "Create lo0/small-2b:256k with a 256k context window", Detail: []string{"num_ctx 262,144"}},
			migrate.Check(served, migrate.Anthropic),
		},
	}
	if session {
		p.Session = &migrate.Session{Program: "claude", Detail: []string{"ANTHROPIC_BASE_URL=http://127.0.0.1:11434", "Set for this session only; your Claude Code settings are not changed."}}
		p.Then = []string{"To keep Coder-30B-A3B in Claude Code on a local model, run lo0 migrate --to claude-code --model coder-30b-a3b --runtime ollama."}
	}
	return p
}

func (f fakeTrier) PlanTry(_ app.Assessment, c app.MigrationChoice) (migrate.Plan, error) {
	return f.plan(c, true), nil
}

func (f fakeTrier) PlanMigration(_ app.Assessment, c app.MigrationChoice) (migrate.Plan, error) {
	return f.plan(c, false), nil
}

func (f fakeTrier) Inventory(app.Assessment) (app.Inventory, error) {
	made := time.Date(2026, 11, 5, 7, 55, 0, 0, time.UTC)
	return app.Inventory{
		DiskBytes: 20 << 30, FreeBytes: 197 << 30,
		Runtimes: []app.InstalledRuntime{
			{ID: runtimes.Ollama, Name: "Ollama", Installed: true, Running: true, Version: "0.34.4", Removable: true, Models: []app.InstalledModel{
				{Name: "coder:30b", Bytes: 18 << 30, CatalogID: "coder-30b-a3b", CatalogName: "Coder-30B-A3B", Quant: "Q4_K_M"},
				{Name: "lo0/coder-30b-a3b:256k", Parent: "coder:30b", MadeByLo0: made, UsedBy: []string{"/Users/dev/.local/bin/claude-local"}},
				{Name: "mystery:2b", Bytes: 2 << 30},
			}},
			{ID: runtimes.LMStudio, Name: "LM Studio"},
		},
		Changes: []migrate.Change{
			{ID: 1, At: made, Runtime: runtimes.Ollama, RuntimeName: "Ollama", Model: "lo0/coder-30b-a3b:256k"},
			{ID: 2, At: made, Path: "/Users/dev/.local/bin/claude-local"},
		},
	}, nil
}

func (f fakeTrier) Harnesses(_ context.Context, a app.Assessment) []app.HarnessView {
	out := make([]app.HarnessView, 0, len(a.Match.Targets))
	for _, t := range a.Match.Targets {
		v := app.HarnessView{
			ID: t.ID, Name: t.Name, Product: map[harness.ID]string{harness.ClaudeCode: "Claude Code", harness.OpenCode: "OpenCode"}[t.ID],
			Fit: t, Recommended: t.ID == a.Match.TargetID, Checked: true,
		}
		if t.ID == harness.ClaudeCode {
			v.Installation = harness.Installation{Installed: true, Version: "2.1.221", Uses: []harness.Use{{Command: "claude", Model: "Claude"}}}
			if f.kept {
				v.Installation.Uses = append(v.Installation.Uses, harness.Use{Command: "claude-local", Model: "Coder-30B-A3B through Ollama", Local: true})
			}
		}
		out = append(out, v)
	}
	return out
}

func (f fakeTrier) PlanInstall(_ app.Assessment, id harness.ID) (migrate.Plan, error) {
	*f.planned = append(*f.planned, app.MigrationChoice{Harness: id})
	return migrate.Plan{Command: "install", Harness: id, HarnessName: "OpenCode", Steps: []migrate.Step{
		{Kind: migrate.KindInstall, Title: "Install OpenCode", Detail: []string{"curl -fsSL https://opencode.ai/install | bash"}},
	}}, nil
}

func (f fakeTrier) PlanRemove(_ app.Assessment, rt runtimes.ID, model string) (migrate.Plan, error) {
	*f.planned = append(*f.planned, app.MigrationChoice{Runtime: rt, Model: model})
	return migrate.Plan{Command: "remove", Runtime: rt, RuntimeName: "Ollama", Model: model, Steps: []migrate.Step{
		{Kind: migrate.KindRuntime, Title: "Remove " + model + " from Ollama", Detail: []string{"ollama rm " + model, "Frees 2 GiB."}},
	}}, nil
}

func (f fakeTrier) PlanPull(_ app.Assessment, c app.MigrationChoice) (migrate.Plan, error) {
	*f.planned = append(*f.planned, c)
	return migrate.Plan{Command: "pull", Model: "General-27B", Steps: []migrate.Step{{Kind: migrate.KindDownload, Title: "Download General-27B", Bytes: 16 << 30}}}, nil
}

func (f fakeTrier) PlanBenchmark(_ app.Assessment, model string) (migrate.Plan, error) {
	*f.planned = append(*f.planned, app.MigrationChoice{Model: model})
	return migrate.Plan{Command: "benchmark", Model: model, Steps: []migrate.Step{
		{Kind: migrate.KindMeasure, Title: "Measure coder:30b at 8k of context"},
		{Kind: migrate.KindMeasure, Title: "Keep the measurements"},
	}}, nil
}

func (f fakeTrier) PlanUndo() (migrate.Plan, error) {
	return migrate.Plan{Undo: true, Steps: []migrate.Step{{Kind: migrate.KindRuntime, Title: "Remove lo0/coder-30b-a3b:256k from Ollama"}}}, nil
}

func (f fakeTrier) Migrate(_ context.Context, p migrate.Plan, observe migrate.Observer) (migrate.Outcome, error) {
	out := migrate.Outcome{States: make([]migrate.State, len(p.Steps))}
	for i, s := range p.Steps {
		if s.Done {
			out.States[i] = migrate.StateAlready
		} else {
			observe(migrate.Event{Step: i, State: migrate.StateRunning})
			observe(migrate.Event{Step: i, State: migrate.StateProgress, Done: 1, Total: 2})
			out.States[i] = migrate.StateDone
		}
		if f.fail != nil && i == 1 {
			out.States[i] = migrate.StateFailed
			return out, f.fail
		}
		observe(migrate.Event{Step: i, State: out.States[i]})
	}
	return out, nil
}

func (f fakeTrier) SessionReport(_ context.Context, a app.Assessment, p migrate.Plan, started, ended time.Time, exit int) (app.SessionReport, error) {
	return app.SessionReport{
		Harness: p.HarnessName, Model: p.Model, Runtime: p.RuntimeName, Window: p.Context,
		Started: started, Ended: ended, ExitCode: exit, Found: true,
		Requests: 23, PeakContext: 61_000, ToolResults: 118, ToolErrors: 6, UsualToolErrors: 0.02,
		Speed: 19, Replies: 40, Estimate: 21,
		Pace:       match.PaceOf(19, 0, a.Match.References),
		References: a.Match.References,
	}, nil
}

// fakeSession is a harness that is never really started.
type fakeSession struct{ program string }

func (fakeSession) Run() error          { return nil }
func (fakeSession) SetStdin(io.Reader)  {}
func (fakeSession) SetStdout(io.Writer) {}
func (fakeSession) SetStderr(io.Writer) {}

func trying(t *testing.T, w, h int, trier fakeTrier) (Model, *[]string) {
	t.Helper()
	var started []string
	m := New(Options{
		Service: fakeAssessor{}, DarkBackground: true, Home: filepath.FromSlash("/Users/dev"), Version: "test",
		Now:     func() time.Time { return tryClock },
		Actions: trier,
		Session: func(s migrate.Session) tea.ExecCommand {
			started = append(started, s.Program)
			return fakeSession{s.Program}
		},
	})
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m = settle(t, next.(Model))
	return press(t, m, "3"), &started
}

// pump runs cmd and feeds its messages back until there are none, or until
// the flow hands the terminal to the harness, which only the Bubble Tea
// runtime can do.
func pump(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	for cmd != nil {
		msg := cmd()
		// A batch pairs the next step with the live race's first frame;
		// the frames only redraw, so the pump follows the step and drops
		// them.
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				if c == nil {
					continue
				}
				if msg = c(); !isFrame(msg) {
					break
				}
			}
			if isFrame(msg) {
				return m
			}
		}
		next, nextCmd := m.Update(msg)
		m = next.(Model)
		cmd = nextCmd
		if m.try.stage == stageReading {
			return m
		}
	}
	return m
}

func isFrame(msg tea.Msg) bool {
	_, ok := msg.(raceTickMsg)
	return ok
}

func hit(t *testing.T, m Model, k string) (Model, tea.Cmd) {
	t.Helper()
	var msg tea.KeyPressMsg
	switch k {
	case "enter":
		msg = tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		msg = tea.KeyPressMsg{Code: tea.KeyEscape}
	default:
		msg = tea.KeyPressMsg{Code: []rune(k)[0], Text: k}
	}
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

func TestTryFromModels(t *testing.T) {
	var planned []app.MigrationChoice
	m, started := trying(t, 120, 36, fakeTrier{planned: &planned})
	m = pump(t, m, func() tea.Cmd { _, c := hit(t, m, "t"); return c }())
	if !m.try.active || m.try.stage != stagePick {
		t.Fatalf("t did not open the harness picker: %+v", m.try)
	}
	golden(t, "wide-try-pick", shot(m))

	m = press(t, m, "enter")
	if m.try.stage != stagePlan || len(planned) != 1 || planned[0].Harness != harness.ClaudeCode || planned[0].Model == "" {
		t.Fatalf("planned %+v, stage %v; want the recommended harness and the selected model", planned, m.try.stage)
	}
	golden(t, "wide-try-plan", shot(m))

	m, cmd := hit(t, m, "enter")
	m = pump(t, m, cmd)
	if m.try.stage != stageReading || len(*started) != 1 || (*started)[0] != "claude" || !m.try.started.Equal(tryClock) {
		t.Fatalf("stage %v, started %v; want the harness handed the terminal after the steps", m.try.stage, *started)
	}

	next, cmd := m.Update(sessionEndedMsg{run: m.try.run, ended: tryClock.Add(14 * time.Minute)})
	m = pump(t, next.(Model), cmd)
	if m.try.stage != stageReport || m.try.report.Requests != 23 {
		t.Fatalf("stage %v, report %+v", m.try.stage, m.try.report)
	}
	view := shot(m)
	for _, want := range []string{"How the session went", "14m 0s · 23 requests · exited normally", "19 tok/s measured", "slower", "Keep this setup"} {
		if !strings.Contains(view, want) {
			t.Errorf("report lacks %q:\n%s", want, view)
		}
	}
	golden(t, "wide-try-report", view)

	m = press(t, m, "enter")
	if m.try.kind != kindKeep || m.try.stage != stagePlan || m.try.plan.Session != nil || planned[1].Harness != harness.ClaudeCode {
		t.Errorf("keep did not plan the migration for the same harness: %+v", m.try)
	}
	if !strings.Contains(shot(m), "Keep Coder-30B-A3B in Claude Code on a local model") {
		t.Errorf("keep plan title:\n%s", shot(m))
	}
	m = press(t, m, "esc")
	if m.try.active {
		t.Error("esc did not close the flow")
	}
}

func TestTryNarrow(t *testing.T) {
	var planned []app.MigrationChoice
	m, _ := trying(t, 80, 24, fakeTrier{planned: &planned})
	m = pump(t, m, func() tea.Cmd { _, c := hit(t, m, "t"); return c }())
	m = press(t, m, "enter")
	golden(t, "narrow-try-plan", shot(m))
}

func TestTryFailureShowsTheOutcome(t *testing.T) {
	var planned []app.MigrationChoice
	m, started := trying(t, 120, 36, fakeTrier{planned: &planned, fail: errors.New("Ollama answered 500")})
	m = pump(t, m, func() tea.Cmd { _, c := hit(t, m, "t"); return c }())
	m = press(t, m, "enter")
	m, cmd := hit(t, m, "enter")
	m = pump(t, m, cmd)
	if m.try.stage != stageOutcome || len(*started) != 0 {
		t.Fatalf("stage %v, started %v; want the failure shown and nothing started", m.try.stage, *started)
	}
	if !strings.Contains(shot(m), "Ollama answered 500") {
		t.Errorf("failure not shown:\n%s", shot(m))
	}
}

func TestTryNeedsATrier(t *testing.T) {
	m := press(t, ready(t, 120, 36), "3")
	m, cmd := hit(t, m, "t")
	if cmd != nil || m.try.active {
		t.Error("t started a try without a trier")
	}
	if strings.Contains(shot(m), "t try") {
		t.Error("the footer offers try without a trier")
	}
}

func TestInstalledScreen(t *testing.T) {
	for _, size := range []struct {
		name string
		w, h int
	}{{"wide", 120, 36}, {"narrow", 80, 24}} {
		t.Run(size.name, func(t *testing.T) {
			var planned []app.MigrationChoice
			m, _ := trying(t, size.w, size.h, fakeTrier{planned: &planned})
			m = press(t, m, "5")
			if m.active != screenMachine {
				t.Fatalf("4 opened screen %v", m.active)
			}
			golden(t, size.name+"-installed-changes", shot(m))
		})
	}
}

func TestRemoveFromInstalled(t *testing.T) {
	var planned []app.MigrationChoice
	m, _ := trying(t, 120, 36, fakeTrier{planned: &planned})
	m = press(t, m, "5", "down", "down")
	m = pump(t, m, func() tea.Cmd { _, c := hit(t, m, "x"); return c }())
	if !m.try.active || m.try.kind != kindRemove || len(planned) != 1 || planned[0].Model != "mystery:2b" {
		t.Fatalf("x did not plan removing the selected model: planned %+v, flow %+v", planned, m.try)
	}
	view := shot(m)
	for _, want := range []string{"Remove mystery:2b", "ollama rm mystery:2b", "Frees 2 GiB.", "apply it"} {
		if !strings.Contains(view, want) {
			t.Errorf("remove plan lacks %q:\n%s", want, view)
		}
	}
	m, cmd := hit(t, m, "enter")
	m = pump(t, m, cmd)
	if m.try.stage != stageOutcome || !m.try.changed {
		t.Fatalf("stage %v, changed %v; want the outcome of a change", m.try.stage, m.try.changed)
	}
	m, cmd = hit(t, m, "esc")
	if m.try.active || m.state != stateLoading || cmd == nil {
		t.Error("closing after a change did not scan the machine again")
	}
}

func TestUndoAndPull(t *testing.T) {
	var planned []app.MigrationChoice
	m, _ := trying(t, 120, 36, fakeTrier{planned: &planned})
	m = press(t, m, "5")
	m = pump(t, m, func() tea.Cmd { _, c := hit(t, m, "u"); return c }())
	if m.try.kind != kindUndo || !strings.Contains(shot(m), "Undo what lo0 changed") {
		t.Fatalf("u did not open the undo plan:\n%s", shot(m))
	}
	m, _ = hit(t, m, "esc")

	// General-27B fits and is not installed.
	m = press(t, m, "3", "down")
	m = pump(t, m, func() tea.Cmd { _, c := hit(t, m, "p"); return c }())
	if m.try.kind != kindPull || !strings.Contains(shot(m), "Download General-27B") {
		t.Fatalf("p did not open the download plan:\n%s", shot(m))
	}
}

func TestHarnessesScreen(t *testing.T) {
	for _, size := range []struct {
		name string
		w, h int
	}{{"wide", 120, 36}, {"narrow", 80, 24}} {
		t.Run(size.name, func(t *testing.T) {
			var planned []app.MigrationChoice
			m, _ := trying(t, size.w, size.h, fakeTrier{planned: &planned, kept: true})
			m = press(t, m, "4")
			golden(t, size.name+"-harnesses-installed", shot(m))
		})
	}
}

func TestHarnessActions(t *testing.T) {
	var planned []app.MigrationChoice
	m, _ := trying(t, 120, 36, fakeTrier{planned: &planned})
	m = press(t, m, "4")
	if !strings.Contains(shot(m), "WHAT MOVES WITH YOU TO CLAUDE CODE") {
		t.Fatalf("the recommended harness is not selected:\n%s", shot(m))
	}

	m = pump(t, m, func() tea.Cmd { _, c := hit(t, m, "t"); return c }())
	if m.try.stage != stagePlan || m.try.kind != kindTry || planned[0].Harness != harness.ClaudeCode {
		t.Fatalf("t did not plan a try in the selected harness: %+v, planned %+v", m.try, planned)
	}
	m = press(t, m, "esc")

	m = pump(t, m, func() tea.Cmd { _, c := hit(t, m, "m"); return c }())
	if m.try.kind != kindKeep || m.try.plan.Session != nil || !strings.Contains(shot(m), "Keep Coder-30B-A3B in Claude Code") {
		t.Fatalf("m did not plan keeping the setup:\n%s", shot(m))
	}
	m = press(t, m, "esc")

	// OpenCode is not installed: it can be installed but not tried.
	m = press(t, m, "right")
	if !strings.Contains(shot(m), "WHAT MOVES WITH YOU TO OPENCODE") || strings.Contains(shot(m), "t try") {
		t.Fatalf("right did not select OpenCode, or offers to try it uninstalled:\n%s", shot(m))
	}
	m = pump(t, m, func() tea.Cmd { _, c := hit(t, m, "i"); return c }())
	if m.try.kind != kindInstall || !strings.Contains(shot(m), "Install OpenCode") || !strings.Contains(shot(m), "opencode.ai/install") {
		t.Fatalf("i did not show the install plan:\n%s", shot(m))
	}
}

func TestHomeWithActions(t *testing.T) {
	for _, size := range []struct {
		name string
		w, h int
	}{{"wide", 120, 36}, {"narrow", 80, 24}} {
		t.Run(size.name, func(t *testing.T) {
			var planned []app.MigrationChoice
			m, _ := trying(t, size.w, size.h, fakeTrier{planned: &planned})
			m = press(t, m, "1")
			golden(t, size.name+"-home-actions", shot(m))
		})
	}
	var planned []app.MigrationChoice
	m, _ := trying(t, 120, 36, fakeTrier{planned: &planned})
	m = press(t, m, "1")
	h := m.screens[screenHome].(home)
	if len(h.steps) == 0 || h.steps[0].Kind != app.StepTry {
		t.Fatalf("first step = %+v, want a try of the downloaded best fit", h.steps)
	}
	m = pump(t, m, func() tea.Cmd { _, c := hit(t, m, "enter"); return c }())
	if m.try.kind != kindTry || m.try.stage != stagePlan || !m.try.preset {
		t.Errorf("the first step did not plan a try: %+v", m.try)
	}
}

func TestBenchmarkFromModels(t *testing.T) {
	var planned []app.MigrationChoice
	m, _ := trying(t, 120, 36, fakeTrier{planned: &planned})
	if !strings.Contains(shot(m), "b measure") {
		t.Fatalf("the footer does not offer to measure the downloaded best fit:\n%s", shot(m))
	}
	m = pump(t, m, func() tea.Cmd { _, c := hit(t, m, "b"); return c }())
	if m.try.kind != kindBenchmark || !strings.Contains(shot(m), "Measure ") || !strings.Contains(shot(m), "on this machine") {
		t.Fatalf("b did not open the benchmark plan:\n%s", shot(m))
	}
	m, cmd := hit(t, m, "enter")
	m = pump(t, m, cmd)
	if m.try.stage != stageOutcome || !m.try.changed {
		t.Errorf("after measuring: stage %v, changed %v; want the outcome and a scan afterwards", m.try.stage, m.try.changed)
	}
}

func TestStoppingADownloadAsksFirst(t *testing.T) {
	m, _ := trying(t, 120, 36, fakeTrier{})
	stops := 0
	m.try = flow{
		active: true,
		stage:  stageRunning,
		plan:   migrate.Plan{Steps: []migrate.Step{{Title: "Download Small-2B at Q8_0", Bytes: 2 << 30}}},
		states: []migrate.State{migrate.StateRunning},
		cancel: func() { stops++ },
	}
	m, _ = hit(t, m, "esc")
	if stops != 0 || !m.try.stopping {
		t.Fatalf("one esc stopped the download (stops %d, asking %v)", stops, m.try.stopping)
	}
	if !strings.Contains(shot(m), "downloading again picks up from there") {
		t.Errorf("the question does not say what is kept:\n%s", shot(m))
	}
	m, _ = hit(t, m, "j")
	if m.try.stopping {
		t.Fatal("another key did not withdraw the question")
	}
	m, _ = hit(t, m, "esc")
	m, _ = hit(t, m, "esc")
	if stops != 1 || !m.try.stopped {
		t.Errorf("two escs: stops %d, stopped %v; want one stop", stops, m.try.stopped)
	}
}

func TestMeasuringDrawsTheRaceLive(t *testing.T) {
	m, _ := trying(t, 120, 36, fakeTrier{})
	m.try = flow{
		active: true,
		kind:   kindBenchmark,
		name:   "Coder-30B-A3B",
		stage:  stageRunning,
		plan:   migrate.Plan{Command: "benchmark", Steps: []migrate.Step{{Kind: migrate.KindMeasure, Title: "Measure coder:30b at 8k of context"}}},
		states: []migrate.State{migrate.StateRunning},
		done:   50, total: 300,
		stepStarted: tryClock, firstToken: tryClock.Add(time.Second), now: tryClock.Add(2 * time.Second),
		ticking: true,
	}
	out := shot(m)
	for _, want := range []string{"THE SAME REPLY, WRITTEN SIDE BY SIDE", "Opus 5.5 · Claude Code", "50 of 300 tokens · 1s"} {
		if !strings.Contains(out, want) {
			t.Errorf("race missing %q:\n%s", want, out)
		}
	}
	next, cmd := m.Update(raceTickMsg{run: m.try.run})
	if m = next.(Model); cmd == nil || !m.try.ticking {
		t.Error("a tick during a measurement did not schedule the next frame")
	}
	m.try.states = []migrate.State{migrate.StateDone}
	next, cmd = m.Update(raceTickMsg{run: m.try.run})
	if m = next.(Model); cmd != nil || m.try.ticking {
		t.Error("the race kept ticking after the measurement ended")
	}
}

// A plan stops between steps, so ctrl+c stops it and quits only once the
// step under way has returned; and a plan the user stopped never starts the
// harness, even when its last step finished.
func TestCtrlCWaitsForARunningPlan(t *testing.T) {
	m, _ := trying(t, 120, 36, fakeTrier{})
	stops := 0
	m.try = flow{
		active: true,
		kind:   kindTry,
		stage:  stageRunning,
		plan:   migrate.Plan{Steps: []migrate.Step{{Title: "Write opencode.json"}}, Session: &migrate.Session{Program: "opencode"}},
		states: []migrate.State{migrate.StateRunning},
		cancel: func() { stops++ },
	}
	m, cmd := hit(t, m, "ctrl+c")
	if stops != 1 || cmd != nil {
		t.Fatalf("ctrl+c: stops %d, cmd %v; want the plan stopped and no quit yet", stops, cmd)
	}
	next, cmd := m.Update(appliedMsg{run: m.try.run, outcome: migrate.Outcome{States: []migrate.State{migrate.StateDone}}})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("lo0 did not quit once the plan returned")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok || m.try.stage == stageReading {
		t.Errorf("after the plan returned: stage %v; want a quit, not the harness", m.try.stage)
	}
}

func TestAStoppedTryNeverStartsTheHarness(t *testing.T) {
	m, _ := trying(t, 120, 36, fakeTrier{})
	m.try = flow{
		active:  true,
		kind:    kindTry,
		stage:   stageRunning,
		stopped: true,
		plan:    migrate.Plan{Steps: []migrate.Step{{Title: "Write opencode.json"}}, Session: &migrate.Session{Program: "opencode"}},
		states:  []migrate.State{migrate.StateRunning},
	}
	next, _ := m.Update(appliedMsg{run: m.try.run, outcome: migrate.Outcome{States: []migrate.State{migrate.StateDone}}})
	if m = next.(Model); m.try.stage != stageOutcome {
		t.Errorf("stage = %v, want the outcome rather than the harness", m.try.stage)
	}
}

// Scrolling stops at the end of the plan, so the first ↑ after holding ↓
// moves the view at once.
func TestPlanScrollStopsAtTheEnd(t *testing.T) {
	m, _ := trying(t, 80, 24, fakeTrier{})
	steps := make([]migrate.Step, 40)
	for i := range steps {
		steps[i] = migrate.Step{Kind: migrate.KindWrite, Title: "Write file " + strconv.Itoa(i)}
	}
	m.try = flow{active: true, kind: kindTry, stage: stagePlan, plan: migrate.Plan{Steps: steps}}
	for range 200 {
		m, _ = hit(t, m, "down")
	}
	cw, ch := m.contentSize()
	end := maxScroll(m.tryLines(cw), ch)
	if end == 0 || m.try.scroll != end {
		t.Fatalf("scroll = %d after holding ↓, want the end, %d", m.try.scroll, end)
	}
	m, _ = hit(t, m, "up")
	if m.try.scroll != end-1 {
		t.Errorf("scroll = %d after one ↑, want %d", m.try.scroll, end-1)
	}
}
