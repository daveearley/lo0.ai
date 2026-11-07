package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"lo0.ai/lo0/internal/match"
)

func TestTopBarSaysTheMachineIsSimulated(t *testing.T) {
	sim, err := match.Simulate(match.HardwareQuery{GPU: "RTX 4090"})
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range [][2]int{{120, 36}, {80, 24}} {
		m := New(Options{Service: fakeAssessor{hardware: &sim}, DarkBackground: true, Home: filepath.FromSlash("/Users/dev"), Version: "test"})
		next, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = settle(t, next.(Model))
		top, _, _ := strings.Cut(ansi.Strip(m.View().Content), "\n")
		if !strings.Contains(top, "simulated") {
			t.Errorf("at %dx%d the top bar does not say the machine is simulated: %q", size[0], size[1], top)
		}
	}
}
