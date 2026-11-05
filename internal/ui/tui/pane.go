package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/ui/theme"
)

// entry is one selectable row in a pane.
type entry struct {
	// group starts a new titled group when it differs from the previous
	// entry's.
	group string
	title string
	// meta is shown right-aligned and may be styled.
	meta string
	// dim renders the title muted, for rows that are informational only.
	dim    bool
	search string
	detail func(width int) string
}

const (
	// splitWidth is the terminal width from which list and details sit side
	// by side; below it, details open over the list.
	splitWidth = 100
	gutter     = 3
)

// pane is a filterable list with a details view for the selected entry.
type pane struct {
	t       theme.Theme
	keys    keyMap
	entries []entry
	// matches are the indices of entries that pass the filter.
	matches []int
	cursor  int
	offset  int

	filter    textinput.Model
	filtering bool
	detail    viewport.Model
	// open is true when the details cover the list on a narrow terminal.
	open bool

	width, height int
}

func newPane(t theme.Theme, keys keyMap, entries []entry, placeholder string) pane {
	in := textinput.New()
	in.Prompt = "/ "
	in.Placeholder = placeholder
	p := pane{t: t, keys: keys, entries: entries, filter: in, detail: viewport.New()}
	p.applyFilter()
	return p
}

func (p pane) split() bool { return p.width >= splitWidth }

func (p pane) listWidth() int {
	if !p.split() {
		return p.width
	}
	return min(max(p.width*2/5, 36), 60)
}

func (p pane) detailWidth() int {
	if !p.split() {
		return p.width
	}
	return p.width - p.listWidth() - gutter
}

func (p *pane) setSize(w, h int) {
	p.width, p.height = w, h
	p.filter.SetWidth(max(p.listWidth()-4, 10))
	p.detail.SetWidth(p.detailWidth())
	p.detail.SetHeight(h)
	p.ensureVisible()
	p.refreshDetail()
}

// capturesKeys reports whether the pane wants every key, as it does while
// the filter is being typed.
func (p pane) capturesKeys() bool { return p.filtering || p.open }

func (p *pane) applyFilter() {
	q := strings.ToLower(strings.TrimSpace(p.filter.Value()))
	p.matches = make([]int, 0, len(p.entries))
	for i, e := range p.entries {
		if q == "" || strings.Contains(strings.ToLower(e.search), q) {
			p.matches = append(p.matches, i)
		}
	}
	p.cursor = min(p.cursor, max(len(p.matches)-1, 0))
	p.offset = 0
	p.ensureVisible()
	p.refreshDetail()
}

func (p *pane) selected() (entry, bool) {
	if len(p.matches) == 0 {
		return entry{}, false
	}
	return p.entries[p.matches[p.cursor]], true
}

func (p *pane) refreshDetail() {
	e, ok := p.selected()
	if !ok || e.detail == nil {
		p.detail.SetContent("")
		return
	}
	p.detail.SetContent(e.detail(p.detailWidth()))
	p.detail.GotoTop()
}

func (p *pane) move(delta int) {
	if len(p.matches) == 0 {
		return
	}
	next := min(max(p.cursor+delta, 0), len(p.matches)-1)
	if next != p.cursor {
		p.cursor = next
		p.ensureVisible()
		p.refreshDetail()
	}
}

func (p pane) update(msg tea.Msg) (pane, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return p.handleKey(msg)
	case tea.MouseWheelMsg:
		m := msg.Mouse()
		if p.split() && m.X >= p.listWidth()+gutter || p.open {
			var cmd tea.Cmd
			p.detail, cmd = p.detail.Update(msg)
			return p, cmd
		}
		switch m.Button {
		case tea.MouseWheelUp:
			p.move(-1)
		case tea.MouseWheelDown:
			p.move(1)
		default:
		}
	}
	return p, nil
}

func (p pane) handleKey(msg tea.KeyPressMsg) (pane, tea.Cmd) {
	if p.filtering {
		switch msg.String() {
		case "esc":
			p.filtering = false
			p.filter.Blur()
			p.filter.SetValue("")
			p.applyFilter()
			return p, nil
		case "enter":
			p.filtering = false
			p.filter.Blur()
			return p, nil
		case "up", "down":
			p.filtering = false
			p.filter.Blur()
			return p.handleKey(msg)
		}
		var cmd tea.Cmd
		p.filter, cmd = p.filter.Update(msg)
		p.applyFilter()
		return p, cmd
	}
	if p.open {
		switch {
		case key.Matches(msg, p.keys.Back), key.Matches(msg, p.keys.Open):
			p.open = false
		default:
			var cmd tea.Cmd
			p.detail, cmd = p.detail.Update(msg)
			return p, cmd
		}
		return p, nil
	}
	switch {
	case key.Matches(msg, p.keys.Up):
		p.move(-1)
	case key.Matches(msg, p.keys.Down):
		p.move(1)
	case key.Matches(msg, p.keys.PageUp):
		p.move(-max(p.height-2, 1))
	case key.Matches(msg, p.keys.PageDown):
		p.move(max(p.height-2, 1))
	case key.Matches(msg, p.keys.Top):
		p.move(-len(p.matches))
	case key.Matches(msg, p.keys.Bottom):
		p.move(len(p.matches))
	case key.Matches(msg, p.keys.ScrollUp):
		p.detail.ScrollUp(3)
	case key.Matches(msg, p.keys.ScrollDown):
		p.detail.ScrollDown(3)
	case key.Matches(msg, p.keys.Filter):
		p.filtering = true
		return p, p.filter.Focus()
	case key.Matches(msg, p.keys.Back):
		if p.filter.Value() != "" {
			p.filter.SetValue("")
			p.applyFilter()
		}
	case key.Matches(msg, p.keys.Open):
		if !p.split() {
			p.open = true
		}
	}
	return p, nil
}

func (p pane) view() string {
	if p.open && !p.split() {
		return p.detail.View()
	}
	list := p.listView()
	if !p.split() {
		return list
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(p.listWidth()).Render(list),
		strings.Repeat(" ", gutter),
		p.detail.View(),
	)
}

// lines lays out the list: group titles, blank separators and one line per
// matching entry. match is the entry's position in matches, or -1.
func (p pane) lines() []int {
	var out []int
	group := ""
	for mi, idx := range p.matches {
		if g := p.entries[idx].group; g != group {
			if len(out) > 0 {
				out = append(out, -1)
			}
			out = append(out, -1)
			group = g
		}
		out = append(out, mi)
	}
	return out
}

func (p pane) headerHeight() int {
	if p.filtering || p.filter.Value() != "" {
		return 2
	}
	return 0
}

// ensureVisible scrolls the list so the cursor is on screen, keeping group
// titles in view when the cursor reaches the top.
func (p *pane) ensureVisible() {
	height := max(p.height-p.headerHeight(), 1)
	at := 0
	for i, m := range p.lines() {
		if m == p.cursor {
			at = i
		}
	}
	if p.cursor == 0 {
		p.offset = 0
		return
	}
	if at < p.offset {
		p.offset = at
	}
	if at >= p.offset+height {
		p.offset = at - height + 1
	}
}

func (p pane) listView() string {
	width := p.listWidth()
	var out []string
	if p.headerHeight() > 0 {
		count := p.t.Muted.Render(format.Int(len(p.matches)) + " of " + format.Int(len(p.entries)))
		out = append(out, p.filter.View()+"  "+count, "")
	}
	if len(p.matches) == 0 {
		return strings.Join(append(out, p.t.Muted.Render("Nothing matches the filter.")), "\n")
	}

	layout := p.lines()
	height := max(p.height-p.headerHeight(), 1)
	end := min(p.offset+height, len(layout))
	for i := p.offset; i < end; i++ {
		m := layout[i]
		switch {
		case m >= 0:
			out = append(out, p.row(p.entries[p.matches[m]], m == p.cursor, width))
		case i+1 < len(layout) && layout[i+1] >= 0:
			out = append(out, p.t.Label.Render(p.entries[p.matches[layout[i+1]]].group))
		default:
			out = append(out, "")
		}
	}
	return strings.Join(out, "\n")
}

func (p pane) row(e entry, selected bool, width int) string {
	marker, title := "  ", p.t.Value.Render(e.title)
	if e.dim {
		title = p.t.Muted.Render(e.title)
	}
	if selected {
		marker = p.t.Accent.Render("› ")
		title = p.t.Accent.Bold(true).Render(e.title)
	}
	room := width - lipgloss.Width(marker) - lipgloss.Width(e.meta) - 2
	title = ansi.Truncate(title, max(room, 4), "…")
	pad := max(width-lipgloss.Width(marker)-lipgloss.Width(title)-lipgloss.Width(e.meta), 1)
	return marker + title + strings.Repeat(" ", pad) + e.meta
}
