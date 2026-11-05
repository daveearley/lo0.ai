package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/ui/report"
)

// harnessScreen is the Harnesses screen: each harness lo0 can set up, with
// what it uses now, and below it everything in the setup and what happens
// to it in the selected harness. Left and right switch harness.
type harnessScreen struct {
	env    env
	views  []app.HarnessView
	target int
	width  int
	height int
	pane   pane
}

// tryInMsg asks the root to try the best model in a harness, keepInMsg to
// make it the setup, and installMsg to install the harness.
type (
	tryInMsg   struct{ harness harness.ID }
	keepInMsg  struct{ harness harness.ID }
	installMsg struct{ harness harness.ID }
)

func newHarnesses(e env) screen {
	s := harnessScreen{env: e, views: e.harnesses}
	if len(s.views) == 0 {
		// Without a way to read the harnesses, the carry-over still shows.
		for _, t := range e.assessment.Match.Targets {
			s.views = append(s.views, app.HarnessView{ID: t.ID, Name: t.Name, Fit: t, Recommended: t.ID == e.assessment.Match.TargetID})
		}
	}
	for i, v := range s.views {
		if v.Recommended {
			s.target = i
		}
	}
	return s.withPane()
}

func (s harnessScreen) withPane() harnessScreen {
	s.pane = newPane(s.env.theme, s.env.keys, featureEntries(s.env, s.current().Fit), "feature, status or area")
	if s.width > 0 {
		s.pane.setSize(s.width, s.paneHeight())
	}
	return s
}

func (s harnessScreen) current() app.HarnessView {
	if s.target < len(s.views) {
		return s.views[s.target]
	}
	return app.HarnessView{}
}

func (s harnessScreen) table() string {
	return report.RenderHarnesses(s.views, s.target, s.env.reportOptions(s.width))
}

func (s harnessScreen) paneHeight() int {
	return max(s.height-strings.Count(s.table(), "\n")-2, 4)
}

func (s harnessScreen) update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case sizeMsg:
		s.width, s.height = msg.width, msg.height
		s.pane.setSize(msg.width, s.paneHeight())
		return s, nil
	case tea.KeyPressMsg:
		if s.pane.capturesKeys() || s.pane.open {
			break
		}
		v := s.current()
		switch msg.String() {
		case "left", "h":
			if len(s.views) > 1 {
				s.target = (s.target + len(s.views) - 1) % len(s.views)
				return s.withPane(), nil
			}
		case "right", "l":
			if len(s.views) > 1 {
				s.target = (s.target + 1) % len(s.views)
				return s.withPane(), nil
			}
		case "t":
			if s.env.try && v.Installation.Installed && s.hasModel() {
				return s, func() tea.Msg { return tryInMsg{harness: v.ID} }
			}
			return s, nil
		case "m":
			if s.env.change && s.hasModel() {
				return s, func() tea.Msg { return keepInMsg{harness: v.ID} }
			}
			return s, nil
		case "i":
			if s.env.change && !v.Installation.Installed && len(s.env.harnesses) > 0 {
				return s, func() tea.Msg { return installMsg{harness: v.ID} }
			}
			return s, nil
		}
	}
	var cmd tea.Cmd
	s.pane, cmd = s.pane.update(msg)
	return s, cmd
}

// hasModel reports whether a model fits, so there is something to try.
func (s harnessScreen) hasModel() bool {
	_, ok := s.env.assessment.Match.Best()
	return ok
}

func (s harnessScreen) view(int, int) string {
	label := s.env.theme.Label.Render("WHAT MOVES WITH YOU TO " + strings.ToUpper(s.current().Name))
	return s.table() + "\n" + label + "\n" + s.pane.view()
}

func (s harnessScreen) capturesKeys() bool { return s.pane.capturesKeys() }

func (s harnessScreen) hints() []hint {
	if s.pane.capturesKeys() || s.pane.open {
		return s.paneHints()
	}
	var h []hint
	if len(s.views) > 1 {
		h = append(h, hint{"←→", "harness"})
	}
	v := s.current()
	if s.env.try && v.Installation.Installed && s.hasModel() {
		h = append(h, hint{"t", "try"})
	}
	if s.env.change && s.hasModel() {
		h = append(h, hint{"m", "set up for good"})
	}
	if s.env.change && !v.Installation.Installed && len(s.env.harnesses) > 0 {
		h = append(h, hint{"i", "install"})
	}
	return append(h, s.paneHints()...)
}

func (s harnessScreen) paneHints() []hint {
	switch {
	case s.pane.filtering:
		return []hint{{"type", "to filter"}, {"enter", "keep"}, {"esc", "clear"}}
	case s.pane.open:
		return []hint{{"↑↓", "scroll"}, {"esc", "back"}}
	default:
		return []hint{{"↑↓", "item"}, {"enter", "details"}, {"/", "search"}}
	}
}
