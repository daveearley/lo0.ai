package progress_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"lo0.ai/lo0/internal/machine"
	"lo0.ai/lo0/internal/runtimes"
	"lo0.ai/lo0/internal/scan"
	"lo0.ai/lo0/internal/ui/progress"
	"lo0.ai/lo0/internal/ui/theme"
	"lo0.ai/lo0/internal/usage"
)

func apply(t *testing.T, m tea.Model, msgs ...tea.Msg) (tea.Model, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, msg := range msgs {
		m, cmd = m.Update(msg)
	}
	return m, cmd
}

func view(m tea.Model) string {
	return ansi.Strip(m.View().Content)
}

func TestViewShowsEachStage(t *testing.T) {
	m, _ := apply(t, progress.New(theme.New(true), nil),
		progress.EventMsg{Stage: scan.StageMachine, Status: scan.StatusRunning},
		progress.EventMsg{Stage: scan.StageMachine, Status: scan.StatusDone},
		progress.EventMsg{Stage: scan.StageSetup, Status: scan.StatusDone, Warnings: 2},
		progress.EventMsg{Stage: scan.StageHistory, Status: scan.StatusRunning, Done: 1204, Total: 2155},
	)
	out := view(m)
	for _, want := range []string{
		"✓ Hardware",
		"· Local runtimes",
		"! Claude Code setup",
		"2 warnings",
		"Session history",
		"1,204 of 2,155 sessions",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q:\n%s", want, out)
		}
	}
	if lines := strings.Count(out, "\n"); lines != len(scan.Stages) {
		t.Errorf("view has %d lines, want %d", lines, len(scan.Stages))
	}
}

func TestStatusEventKeepsProgressCounts(t *testing.T) {
	m, _ := apply(t, progress.New(theme.Plain(), nil),
		progress.EventMsg{Stage: scan.StageHistory, Status: scan.StatusRunning, Done: 10, Total: 20},
		progress.EventMsg{Stage: scan.StageHistory, Status: scan.StatusRunning},
	)
	if !strings.Contains(view(m), "10 of 20 sessions") {
		t.Errorf("progress counts lost:\n%s", view(m))
	}
}

func TestDoneClearsViewAndQuits(t *testing.T) {
	m, cmd := apply(t, progress.New(theme.Plain(), nil), progress.DoneMsg{})
	if view(m) != "" {
		t.Errorf("view after done = %q, want empty", view(m))
	}
	if cmd == nil {
		t.Fatal("done returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("done did not quit")
	}
}

func TestCtrlCCancelsScan(t *testing.T) {
	cancelled := false
	m, cmd := apply(t, progress.New(theme.Plain(), func() { cancelled = true }),
		tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl},
	)
	if !cancelled || !m.(progress.Model).Interrupted() {
		t.Error("ctrl+c did not cancel the scan")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("ctrl+c did not quit")
	}
}

func TestWithThemeKeepsProgress(t *testing.T) {
	m, _ := apply(t, progress.New(theme.Plain(), nil), progress.EventMsg{Stage: scan.StageMachine, Status: scan.StatusDone})
	restyled := m.(progress.Model).WithTheme(theme.New(false))
	if !strings.Contains(view(restyled), "✓ Hardware") {
		t.Errorf("restyling lost progress:\n%s", view(restyled))
	}
}

func TestOtherKeysAreIgnored(t *testing.T) {
	m, cmd := apply(t, progress.New(theme.Plain(), nil), tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd != nil || m.(progress.Model).Interrupted() {
		t.Error("an ordinary key changed the scan")
	}
}

func TestFinishedStagesSayWhatTheyFound(t *testing.T) {
	m, _ := apply(t, progress.New(theme.Plain(), nil),
		progress.EventMsg{Stage: scan.StageMachine, Status: scan.StatusDone, Found: scan.Found{
			Machine: machine.Profile{CPU: machine.CPU{Model: "Apple M5 Pro"}, Memory: machine.Memory{TotalBytes: 64 << 30}},
		}},
		progress.EventMsg{Stage: scan.StageRuntimes, Status: scan.StatusDone, Found: scan.Found{
			Runtimes: []runtimes.Runtime{{Name: "Ollama", Installed: true, Models: make([]runtimes.Model, 3)}, {Name: "LM Studio"}},
		}},
		progress.EventMsg{Stage: scan.StageHistory, Status: scan.StatusDone, Found: scan.Found{
			Usage: usage.Profile{Sessions: 483, Turns: 54413},
		}},
	)
	out := view(m)
	for _, want := range []string{"Apple M5 Pro · 64 GiB", "Ollama 3 models", "483 sessions · 54,413 turns"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "LM Studio") {
		t.Errorf("a runtime that is not installed is listed:\n%s", out)
	}
}
