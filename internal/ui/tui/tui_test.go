package tui

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/claudecode/install"
	"lo0.ai/lo0/internal/claudecode/setup"
	"lo0.ai/lo0/internal/cost"
	"lo0.ai/lo0/internal/harness/claude"
	"lo0.ai/lo0/internal/harness/opencode"
	"lo0.ai/lo0/internal/machine"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/runtimes"
	"lo0.ai/lo0/internal/scan"
	"lo0.ai/lo0/internal/usage"
)

var update = flag.Bool("update", false, "rewrite golden files")

func harnesses() []match.Harness {
	return []match.Harness{claude.Harness{}, opencode.Harness{}}
}

func fixtureCatalog() *catalog.Catalog {
	scores := map[string][]catalog.Benchmark{
		"Coder-30B-A3B":  {{Key: "swe-bench-verified", Name: "SWE-bench Verified", Score: "61.0"}, {Name: "MCPMark", Score: "37.0"}},
		"Hybrid-80B-A3B": {{Key: "swe-bench-verified", Name: "SWE-bench Verified", Setting: "high", Score: "70.6"}},
		"General-27B":    {{Key: "bfcl-v4", Name: "BFCL v4", Score: "66.1"}},
	}
	model := func(id string, tier catalog.Tier, params, active int64, layers int, ollama ...string) catalog.Model {
		return catalog.Model{
			ID: id, Name: id, Family: "test", Repo: "lab/" + id, License: "apache-2.0", Released: "2026-01-01",
			Params: params, ActiveParams: active, ContextLength: 262144, Ollama: ollama,
			Attention: catalog.Attention{FullLayers: layers, FullKV: 1024},
			Quants: []catalog.Quant{
				{Name: "Q8_0", Bytes: params * 85 / 80, BitsPerWeight: 8.5},
				{Name: "Q4_K_M", Bytes: params * 485 / 800, BitsPerWeight: 4.85},
			},
			Tier: tier, TierNote: "Provisional: test tier.",
			Profile: catalog.Profile{
				Summary:    "A " + id + " model built for testing lo0.",
				GoodAt:     []string{"Tool calling in long agent loops", "Small active parameter count"},
				WatchOut:   []string{"Needs its own tool-call parser"},
				Benchmarks: scores[id],
				Card:       "https://huggingface.co/lab/" + id,
			},
		}
	}
	return &catalog.Catalog{Generated: "2026-11-04", Models: []catalog.Model{
		model("Coder-30B-A3B", catalog.TierA, 30_500_000_000, 3_400_000_000, 48, "coder:30b"),
		model("Hybrid-80B-A3B", catalog.TierA, 79_700_000_000, 3_900_000_000, 12),
		model("General-27B", catalog.TierB, 27_800_000_000, 27_800_000_000, 16),
		model("Giant-480B", catalog.TierA, 480_000_000_000, 35_000_000_000, 62),
	}}
}

func fixtureScan() scan.Result {
	peaks := make([]int, 0, 50)
	for i := range 50 {
		peaks = append(peaks, 15_000+i*8_000)
	}
	home := filepath.FromSlash("/Users/dev")
	return scan.Result{
		GeneratedAt: time.Date(2026, 11, 4, 18, 0, 0, 0, time.UTC),
		Machine: machine.Profile{
			OS:     machine.OS{GOOS: "darwin", Arch: "arm64", Version: "27.0.1"},
			CPU:    machine.CPU{Model: "Apple M5 Pro", PhysicalCores: 18, LogicalCores: 18},
			Memory: machine.Memory{TotalBytes: 64 << 30, Unified: true},
			GPUs:   []machine.GPU{{Name: "Apple M5 Pro", Vendor: machine.VendorApple, Cores: 20, API: "Metal 4", Unified: true}},
		},
		Runtimes: []runtimes.Runtime{{
			ID: runtimes.Ollama, Name: "Ollama", Installed: true, Service: true, Running: true, Version: "0.34.4", Endpoint: "http://127.0.0.1:11434",
			Models: []runtimes.Model{
				{Name: "coder:30b", Quantization: "Q4_K_M", SizeBytes: 18_500_000_000, ContextLength: 262144, Capabilities: []runtimes.Capability{"tools"}},
				{Name: "hf.co/someone/tiny:Q4_K_M", Parameters: "4B", Quantization: "Q4_K_M", SizeBytes: 2_500_000_000},
			},
		}},
		Setup: setup.Inventory{
			Instructions: []setup.InstructionFile{{Origin: setup.Origin{Scope: setup.ScopeUser, Path: filepath.Join(home, ".claude", "CLAUDE.md")}, Kind: setup.InstructionClaudeMD, Lines: 20}},
			MCPServers:   []setup.MCPServer{{Name: "github", Transport: setup.TransportHTTP}},
			Hooks:        []setup.Hook{{Event: "PreToolUse", Type: "command"}, {Event: "Notification", Type: "command"}},
			Account:      setup.Account{Plan: setup.PlanMax, Tier: "5x"},
		},
		Usage: usage.Profile{
			Speeds: []usage.Speed{
				{Harness: "Claude Code", Model: "claude-opus-5-5", Replies: 22_974, Recent: 19_500, TokensPerSecond: 96, ReplyTokens: 565, ReplySeconds: 6.1},
				{Harness: "Codex", Model: "gpt-5.5", Replies: 1_023, TokensPerSecond: 45, ReplyTokens: 405, ReplySeconds: 9.6},
			},
			Sessions: 50, Turns: 4000,
			Context:    usage.Context{P50: peaks[25], P90: peaks[45], Max: peaks[49], Peaks: peaks},
			MCPServers: []usage.Tally{{Name: "Claude_Browser", Count: 300, Sessions: 9}, {Name: "github", Count: 40, Sessions: 5}},
			First:      time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC),
			Last:       time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC),
			Usage:      []usage.ModelUsage{{Model: "claude-opus-5-5", Tokens: usage.Tokens{Input: 2e6, CacheWrite1h: 40e6, CacheRead: 900e6, Output: 6e6}}},
			Records:    records(peaks),
			Tools:      []usage.Tally{{Name: "Bash", Count: 900, Sessions: 50}, {Name: "Edit", Count: 210, Sessions: 30}},
			Requests: usage.Requests{
				Count: 400, ToolCalls: usage.Spread{P50: 4, P90: 31, Max: 120},
				Output: usage.Spread{P50: 2100, P90: 9000}, Seconds: usage.Spread{P50: 41, P90: 380},
			},
			Interruptions: usage.Interruptions{LimitHits: 2, LimitSessions: 2, Retries: 40, NetworkRetries: 12, Sessions: 9},
			Changes:       usage.Changes{LinesAdded: 12400, LinesRemoved: 3100, Files: 140, PullRequests: 6, PRSessions: 5},
			Format:        usage.Format{Responses: 4000, Versions: []usage.Tally{{Name: "2.1.317", Sessions: 30}, {Name: "2.1.300", Sessions: 20}}},
		},
		Claude: install.Installs{App: "2.1.317", Latest: "2.1.318"},
	}
}

// records spreads the fixture's usage over one record per session, a day
// apart, ending on 1 November.
func records(peaks []int) []usage.SessionRecord {
	out := make([]usage.SessionRecord, len(peaks))
	end := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	for i, p := range peaks {
		out[i] = usage.SessionRecord{
			Start:       end.AddDate(0, 0, i-len(peaks)+1),
			PeakContext: p,
			Repo:        repoFor(i),
			Usage:       []usage.ModelUsage{{Model: "claude-opus-5-5", Tokens: usage.Tokens{Input: 40e3, CacheWrite1h: 800e3, CacheRead: 18e6, Output: 120e3}}},
		}
	}
	return out
}

// repoFor spreads the fixture's sessions over two projects and a few
// sessions outside any repository.
func repoFor(i int) string {
	switch {
	case i%5 == 4:
		return ""
	case i%3 == 0:
		return filepath.FromSlash("/Users/dev/code/site")
	default:
		return filepath.FromSlash("/Users/dev/code/app")
	}
}

type fakeAssessor struct {
	err   error
	block bool
}

func (f fakeAssessor) Assess(ctx context.Context, observe scan.Observer) (app.Assessment, error) {
	for _, stage := range scan.Stages {
		observe(scan.Event{Stage: stage, Status: scan.StatusRunning})
		observe(scan.Event{Stage: stage, Status: scan.StatusDone})
	}
	if f.block {
		<-ctx.Done()
		return app.Assessment{}, ctx.Err()
	}
	if f.err != nil {
		return app.Assessment{}, f.err
	}
	res := fixtureScan()
	m := match.Match(res, fixtureCatalog(), harnesses())
	best, _ := m.Best()
	local := cost.Local{Model: best.Model.Name, Window: best.Best.MaxContext, TokensPerSecond: best.Best.TokensPerSecond}
	return app.Assessment{Scan: res, Match: m, Cost: cost.Estimate(res.Usage, res.Setup.Account, local)}, nil
}

// settle runs the assessment to completion, feeding every message back into
// the model the way the Bubble Tea runtime would.
func settle(t *testing.T, m Model) Model {
	t.Helper()
	cmd := m.start()
	for cmd != nil {
		msg := cmd()
		next, nextCmd := m.Update(msg)
		m = next.(Model)
		cmd = nextCmd
		if _, done := msg.(assessedMsg); done {
			break
		}
	}
	return m
}

func ready(t *testing.T, w, h int) Model {
	t.Helper()
	m := New(Options{Service: fakeAssessor{}, DarkBackground: true, Home: filepath.FromSlash("/Users/dev"), Version: "test"})
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m = settle(t, next.(Model))
	if m.state != stateReady {
		t.Fatalf("state = %v, err %v", m.state, m.err)
	}
	return m
}

func press(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		var msg tea.KeyPressMsg
		switch k {
		case "tab":
			msg = tea.KeyPressMsg{Code: tea.KeyTab}
		case "shift+tab":
			msg = tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
		case "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		case "down":
			msg = tea.KeyPressMsg{Code: tea.KeyDown}
		case "up":
			msg = tea.KeyPressMsg{Code: tea.KeyUp}
		case "ctrl+c":
			msg = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
		case "ctrl+k":
			msg = tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl}
		case "space":
			msg = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
		default:
			r := []rune(k)[0]
			msg = tea.KeyPressMsg{Code: r, Text: k}
		}
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

func shot(m Model) string {
	return ansi.Strip(m.View().Content)
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	// Fixtures use Unix paths and the renderers join them with the host's
	// separator, so goldens are kept and compared in slash form.
	got = filepath.ToSlash(got)
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading golden file (run go test -update): %v", err)
	}
	if got != string(want) {
		t.Errorf("screen differs from %s; run go test ./internal/ui/tui -update and review the diff\n\ngot:\n%s", path, got)
	}
}

func TestScreens(t *testing.T) {
	sizes := []struct {
		name string
		w, h int
	}{{"wide", 120, 36}, {"narrow", 80, 24}}
	for _, size := range sizes {
		base := ready(t, size.w, size.h)
		screens := map[string]Model{
			"home":          base,
			"models":        press(t, base, "2"),
			"models-all":    press(t, base, "2", "f", "f"),
			"model-full":    press(t, base, "2", "enter"),
			"compare":       press(t, base, "2", "c"),
			"harnesses":     press(t, base, "3"),
			"installed":     press(t, base, "4"),
			"claude":        press(t, base, "5"),
			"palette":       press(t, base, "ctrl+k"),
			"palette-query": press(t, base, "ctrl+k", "h", "y", "b"),
			"help":          press(t, base, "?"),
		}
		for name, m := range screens {
			t.Run(size.name+"/"+name, func(t *testing.T) {
				out := shot(m)
				lines := strings.Split(out, "\n")
				if len(lines) != size.h {
					t.Errorf("screen is %d lines, window is %d", len(lines), size.h)
				}
				for i, line := range lines {
					if w := ansi.StringWidth(line); w > size.w {
						t.Errorf("line %d is %d columns wide, window is %d: %q", i+1, w, size.w, line)
					}
				}
				golden(t, size.name+"-"+name, out)
			})
		}
	}
}

func TestViewIsFullScreen(t *testing.T) {
	v := ready(t, 100, 30).View()
	if !v.AltScreen || v.MouseMode != tea.MouseModeCellMotion || v.WindowTitle != "lo0" {
		t.Errorf("view = alt %v, mouse %v, title %q", v.AltScreen, v.MouseMode, v.WindowTitle)
	}
}

func TestNavigation(t *testing.T) {
	m := ready(t, 120, 30)
	if m = press(t, m, "tab"); m.active != screenModels {
		t.Errorf("tab went to %v", m.active)
	}
	if m = press(t, m, "shift+tab", "shift+tab"); m.active != screenClaude {
		t.Errorf("shift+tab twice from Models = %v, want wrap to Your Claude", m.active)
	}
	if m = press(t, m, "2"); m.active != screenModels {
		t.Errorf("2 went to %v", m.active)
	}
	if !strings.Contains(shot(m), "What can you run, and how well?") {
		t.Error("screen title missing")
	}
}

func TestSidebarAndBreadcrumb(t *testing.T) {
	wide := shot(press(t, ready(t, 120, 30), "2"))
	if !strings.Contains(wide, "▌ 2 Models") || !strings.Contains(wide, "5 Your Claude") {
		t.Errorf("wide layout has no sidebar:\n%s", wide)
	}
	narrow := shot(press(t, ready(t, 80, 24), "2"))
	if !strings.Contains(narrow, "lo0  ›  Models") || strings.Contains(narrow, "5 Your Claude") {
		t.Errorf("narrow layout should show a breadcrumb, not a sidebar:\n%s", narrow)
	}
}

func TestHomeActions(t *testing.T) {
	// Without a way to change anything, Home offers only what it can do:
	// comparing and browsing.
	m := ready(t, 120, 36)
	h := m.screens[screenHome].(home)
	if len(h.steps) != 2 || h.steps[0].Kind != app.StepCompare || h.steps[1].Kind != app.StepBrowse {
		t.Fatalf("steps = %+v", h.steps)
	}
	m = follow(t, m)
	if m.active != screenModels {
		t.Fatalf("first home step went to %v", m.active)
	}
	if mm := m.screens[screenModels].(models); mm.mode != modeCompare || len(mm.marked) != 2 {
		t.Errorf("compare should open on the best model and the runner-up, mode %v, marked %v", mm.mode, mm.marked)
	}
}

// follow runs the command a key press produced, as the runtime would.
func follow(t *testing.T, m Model) Model {
	t.Helper()
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		return m
	}
	next, _ := m.Update(cmd())
	return next.(Model)
}

func TestPaletteFindsAModel(t *testing.T) {
	m := press(t, ready(t, 120, 36), "ctrl+k", "h", "y", "b", "r")
	if !m.palette.open || len(m.palette.matches) != 1 {
		t.Fatalf("palette open %v with %d matches", m.palette.open, len(m.palette.matches))
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.palette.open || m.active != screenModels {
		t.Fatalf("enter did not navigate: open %v, active %v", m.palette.open, m.active)
	}
	if r, _ := m.screens[screenModels].(models).selected(); r.id != "Hybrid-80B-A3B" {
		t.Errorf("selected %q, want Hybrid-80B-A3B", r.id)
	}
	if m = press(t, press(t, m, "ctrl+k"), "esc"); m.palette.open {
		t.Error("esc did not close the palette")
	}
}

func TestQWhileTypingIsText(t *testing.T) {
	m := press(t, ready(t, 120, 36), "2", "/", "q")
	mm := m.screens[screenModels].(models)
	if !mm.typing || mm.query.Value() != "q" {
		t.Errorf("q while searching should type, query %q typing %v", mm.query.Value(), mm.typing)
	}
	m = press(t, press(t, ready(t, 120, 36), "ctrl+k"), "q")
	if !m.palette.open || m.palette.input.Value() != "q" {
		t.Error("q in the palette should type")
	}
}

func TestModelsFilterSortAndMark(t *testing.T) {
	m := press(t, ready(t, 120, 36), "2")
	mm := m.screens[screenModels].(models)
	fits, installed, all := mm.counts()
	if len(mm.visible) != fits || fits == 0 || installed != 2 || all != 5 {
		t.Fatalf("counts fits %d installed %d all %d, visible %d", fits, installed, all, len(mm.visible))
	}
	if mm = press(t, m, "f").screens[screenModels].(models); mm.filter != filterInstalled || len(mm.visible) != 2 {
		t.Errorf("f should show the 2 installed models, got %d", len(mm.visible))
	}
	sorted := press(t, m, "s", "s").screens[screenModels].(models)
	if sorted.sort != sortSpeed {
		t.Fatalf("sort = %v, want speed", sorted.sort)
	}
	first, second := sorted.rows[sorted.visible[0]], sorted.rows[sorted.visible[1]]
	if first.candidate.Best.TokensPerSecond < second.candidate.Best.TokensPerSecond {
		t.Errorf("speed sort put %s before the faster %s", first.name, second.name)
	}
	marked := press(t, m, "space", "down", "space", "c").screens[screenModels].(models)
	if len(marked.marked) != 2 || marked.mode != modeCompare {
		t.Errorf("marked %v, mode %v", marked.marked, marked.mode)
	}
	if out := ansi.Strip(marked.full.View()); !strings.Contains(out, "Sessions that fit") {
		t.Errorf("compare view:\n%s", out)
	}
	if back := press(t, press(t, m, "c"), "esc").screens[screenModels].(models); back.mode != modeList {
		t.Error("esc did not leave compare")
	}
}

func TestModelDetailFollowsSelection(t *testing.T) {
	m := press(t, ready(t, 120, 36), "2")
	first := ansi.Strip(m.screens[screenModels].(models).panel.View())
	m = press(t, m, "down")
	mm := m.screens[screenModels].(models)
	second := ansi.Strip(mm.panel.View())
	r, _ := mm.selected()
	if first == second || !strings.Contains(second, r.name) {
		t.Errorf("detail did not follow the selection to %s:\n%s", r.name, second)
	}
	for _, want := range []string{"GOOD AT", "WATCH OUT FOR", "ON THIS MACHINE", "Memory", "Sessions", "Writing", "Reading"} {
		if !strings.Contains(second, want) {
			t.Errorf("detail missing %q", want)
		}
	}
}

func TestQuitCancelsTheScan(t *testing.T) {
	m := New(Options{Service: fakeAssessor{block: true}})
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if m.ctx.Err() == nil {
		t.Error("quitting did not cancel the scan")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("q did not quit")
	}
}

func TestFailureAndRetry(t *testing.T) {
	m := New(Options{Service: fakeAssessor{err: errors.New("disk on fire")}})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = settle(t, next.(Model))
	if m.state != stateFailed || !strings.Contains(shot(m), "disk on fire") {
		t.Fatalf("failure not shown:\n%s", shot(m))
	}
	m.opts.Service = fakeAssessor{}
	if m = press(t, m, "r"); m.state != stateLoading {
		t.Fatal("r did not restart the scan")
	}
	if m = settle(t, m); m.state != stateReady {
		t.Errorf("retry did not succeed: %v", m.err)
	}
}

func TestStaleMessagesAreIgnored(t *testing.T) {
	m := ready(t, 100, 30)
	stale := m.run
	next, _ := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m = next.(Model)
	next, _ = m.Update(assessedMsg{run: stale, err: context.Canceled})
	if next.(Model).state != stateLoading {
		t.Error("a cancelled scan's result replaced the new scan")
	}
}

func TestTinyWindow(t *testing.T) {
	if out := shot(ready(t, 40, 10)); !strings.Contains(out, "at least") {
		t.Errorf("tiny window not explained: %q", out)
	}
}

func TestBackgroundChangeRestyles(t *testing.T) {
	m := ready(t, 100, 30)
	next, _ := m.Update(tea.BackgroundColorMsg{Color: lightColor{}})
	if next.(Model).opts.DarkBackground {
		t.Error("a light background was not picked up")
	}
}

type lightColor struct{}

func (lightColor) RGBA() (r, g, b, a uint32) { return 0xffff, 0xffff, 0xffff, 0xffff }

func TestAtomsNeverSplitAnItem(t *testing.T) {
	got := atoms([]string{"SWE-bench Verified 73.4", "Terminal-Bench 2.0 51.5", "MCPMark 37.0"}, " · ", 50, "  ")
	want := "SWE-bench Verified 73.4 · Terminal-Bench 2.0 51.5\n  MCPMark 37.0"
	if got != want {
		t.Errorf("atoms =\n%s\nwant\n%s", got, want)
	}
}

func TestSessionLimitCoversTheLongestSession(t *testing.T) {
	ctx := usage.Context{Peaks: []int{10, 20, 900}}
	if got := sessionLimit(ctx, 100); got != 900 {
		t.Errorf("sessionLimit = %d, want the longest session", got)
	}
	if got := sessionLimit(usage.Context{Peaks: []int{10}}, 800); got != 900 {
		t.Errorf("sessionLimit = %d, want a little past the window", got)
	}
}

func TestHomeShowsWhatUsageIsWorth(t *testing.T) {
	m := ready(t, 120, 36)
	if out := shot(m); !strings.Contains(out, "YOUR CLAUDE") || !strings.Contains(out, "at API prices") {
		t.Errorf("home does not say what the history is worth:\n%s", out)
	}
	page := m.screens[screenClaude].(page)
	if content := ansi.Strip(page.render(100)); !strings.Contains(content, "What your Claude Code use is worth") || !strings.Contains(content, " / 100") {
		t.Errorf("Your Claude lacks the readiness score or what the history is worth:\n%s", content)
	}
}

func TestExportSavesARedactedReport(t *testing.T) {
	saved := map[string]string{"lo0-report-2026-11-04.txt": "an older report"}
	opts := Options{
		Service: fakeAssessor{}, DarkBackground: true, Home: filepath.FromSlash("/Users/dev"),
		Now: func() time.Time { return time.Date(2026, 11, 4, 20, 0, 0, 0, time.UTC) },
		Save: func(name string, data []byte) error {
			if _, ok := saved[name]; ok {
				return fmt.Errorf("creating %s: %w", name, fs.ErrExist)
			}
			saved[name] = string(data)
			return nil
		},
	}
	next, _ := New(opts).Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	m := settle(t, next.(Model))

	m = press(t, m, "e")
	if !m.exporting || !strings.Contains(shot(m), "lo0-report-2026-11-04.txt") {
		t.Fatalf("export confirmation not shown:\n%s", shot(m))
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("enter did not save")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)

	report, ok := saved["lo0-report-2026-11-04-2.txt"]
	if !ok || saved["lo0-report-2026-11-04.txt"] != "an older report" {
		t.Fatalf("saved = %v, want a new file beside the existing one", slices.Sorted(maps.Keys(saved)))
	}
	if strings.Contains(report, "/Users/dev") || strings.Contains(report, "\x1b[") || !strings.Contains(report, "project 1") {
		t.Errorf("saved report is not plain and redacted:\n%s", report)
	}
	if !strings.Contains(shot(m), "Saved lo0-report-2026-11-04-2.txt") {
		t.Errorf("footer does not confirm the save:\n%s", shot(m))
	}
	if m = press(t, m, "down"); strings.Contains(shot(m), "Saved") {
		t.Error("save confirmation stayed after the next key")
	}
}

func TestExportCanBeCancelledAndNeedsSave(t *testing.T) {
	if m := press(t, ready(t, 120, 36), "e"); m.exporting {
		t.Error("export offered without a way to save")
	}
	opts := Options{Service: fakeAssessor{}, Now: time.Now, Save: func(string, []byte) error {
		t.Error("saved after cancelling")
		return nil
	}}
	next, _ := New(opts).Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	m := press(t, settle(t, next.(Model)), "e", "esc")
	if m.exporting {
		t.Error("esc did not cancel the export")
	}
}
