package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"lo0.ai/lo0/internal/runtimes"
	"lo0.ai/lo0/internal/ui/report"
)

// installed is the This machine screen: what is on this machine for running
// models, with remove and undo, above the hardware it runs on.
type installed struct {
	env    env
	rows   []report.InventoryRow
	cursor int
	width  int
	vp     viewport.Model
}

func newInstalled(e env) screen {
	return installed{env: e, rows: report.InventoryRows(e.inventory), vp: viewport.New()}
}

// removeMsg asks the root to plan removing a model.
type removeMsg struct {
	runtime runtimes.ID
	model   string
}

// undoMsg asks the root to plan undoing everything lo0 changed.
type undoMsg struct{}

func (s installed) selected() (report.InventoryRow, bool) {
	if s.cursor < 0 || s.cursor >= len(s.rows) {
		return report.InventoryRow{}, false
	}
	return s.rows[s.cursor], true
}

func (s installed) update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case sizeMsg:
		s.width = msg.width
		s.vp.SetWidth(msg.width)
		s.vp.SetHeight(msg.height)
		s.refresh()
		return s, nil
	case tea.KeyPressMsg:
		k := s.env.keys
		switch {
		case key.Matches(msg, k.Up):
			s.cursor = max(s.cursor-1, 0)
		case key.Matches(msg, k.Down):
			s.cursor = min(s.cursor+1, len(s.rows)-1)
		case key.Matches(msg, k.Top):
			s.cursor = 0
		case key.Matches(msg, k.Bottom):
			s.cursor = len(s.rows) - 1
		case msg.String() == "x" && s.env.change:
			if r, ok := s.selected(); ok && r.Runtime.Removable {
				return s, func() tea.Msg { return removeMsg{runtime: r.Runtime.ID, model: r.Model.Name} }
			}
			return s, nil
		case msg.String() == "u" && s.env.change && len(s.env.inventory.Changes) > 0:
			return s, func() tea.Msg { return undoMsg{} }
		default:
			var cmd tea.Cmd
			s.vp, cmd = s.vp.Update(msg)
			return s, cmd
		}
		s.refresh()
		return s, nil
	}
	return s, nil
}

// refresh redraws the content and keeps the selected model in view.
func (s *installed) refresh() {
	o := s.env.reportOptions(s.width)
	// Without a way to change anything there is no inventory, so the
	// machine and its runtimes are shown as the scan found them.
	content := report.RenderSection(report.SectionMachine, s.env.assessment, o)
	if s.env.change {
		content = report.RenderInventory(s.env.inventory, s.cursor, o)
		if s.env.inventoryErr != nil {
			content = s.env.theme.Bad.Render("Could not read what lo0 changed: "+s.env.inventoryErr.Error()) + "\n" + content
		}
		content += "\n" + report.RenderSection(report.SectionHardware, s.env.assessment, o)
	}
	s.vp.SetContent(content)
	for i, line := range strings.Split(content, "\n") {
		if !strings.HasPrefix(ansi.Strip(line), "▌") {
			continue
		}
		switch top := s.vp.YOffset(); {
		case i < top:
			s.vp.SetYOffset(i)
		case i >= top+s.vp.Height():
			s.vp.SetYOffset(i - s.vp.Height() + 1)
		}
		break
	}
}

func (s installed) view(int, int) string { return s.vp.View() }

func (s installed) hints() []hint {
	var h []hint
	if len(s.rows) > 0 {
		h = append(h, hint{"↑↓", "model"})
	}
	if r, ok := s.selected(); ok && r.Runtime.Removable && s.env.change {
		h = append(h, hint{"x", "remove"})
	}
	if s.env.change && len(s.env.inventory.Changes) > 0 {
		h = append(h, hint{"u", "undo what lo0 changed"})
	}
	return append(h, hint{"pgup pgdn", "scroll"})
}

func (installed) capturesKeys() bool { return false }
