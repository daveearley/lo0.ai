package tui

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/runtimes"
	"lo0.ai/lo0/internal/ui/report"
	"lo0.ai/lo0/internal/ui/theme"
)

type modelFilter int

const (
	filterFits modelFilter = iota
	filterInstalled
	filterAll
)

type modelSort int

const (
	sortBest modelSort = iota
	sortSessions
	sortSpeed
	sortMemory
	sortName
)

var sortNames = []string{"best for you", "sessions", "speed", "memory", "name"}

type modelMode int

const (
	modeList modelMode = iota
	modeFull
	modeCompare
)

// modelRow is one model in the list: a candidate that fits, one that is too
// large, or an installed model lo0 has no data for.
type modelRow struct {
	id        string
	name      string
	search    string
	candidate *match.Candidate
	tooLarge  *match.TooLarge
	unknown   *runtimes.Model
	installed bool
	rank      int
}

// compareTopMsg opens the comparison of the given models, or of the best
// ones when it names none.
type compareTopMsg struct{ ids []string }

type selectModelMsg struct{ id string }

type models struct {
	env     env
	rows    []modelRow
	visible []int
	cursor  int
	offset  int
	filter  modelFilter
	sort    modelSort
	query   textinput.Model
	typing  bool
	marked  []string
	mode    modelMode
	panel   viewport.Model
	full    viewport.Model
	width   int
	height  int
}

func newModels(e env) screen {
	m := e.assessment.Match
	s := models{env: e, panel: viewport.New(), full: viewport.New()}
	s.query = textinput.New()
	s.query.Prompt = "/ "
	s.query.Placeholder = "name, family or lab"
	for i := range m.Candidates {
		c := &m.Candidates[i]
		s.rows = append(s.rows, modelRow{
			id: c.Model.ID, name: c.Model.Name, candidate: c, installed: c.Installed != nil, rank: i,
			search: strings.ToLower(c.Model.Name + " " + c.Model.Family + " " + c.Model.Repo + " " + c.Model.Profile.Summary),
		})
	}
	for i := range m.TooLarge {
		t := &m.TooLarge[i]
		s.rows = append(s.rows, modelRow{
			id: t.Model.ID, name: t.Model.Name, tooLarge: t, rank: len(m.Candidates) + i,
			search: strings.ToLower(t.Model.Name + " " + t.Model.Family + " " + t.Model.Repo),
		})
	}
	for i := range m.Uncatalogued {
		u := &m.Uncatalogued[i]
		s.rows = append(s.rows, modelRow{
			id: "installed:" + u.Name, name: shortModelName(u.Name), unknown: u, installed: true, rank: len(s.rows),
			search: strings.ToLower(u.Name),
		})
	}
	s.apply()
	return s
}

func shortModelName(name string) string {
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	return strings.TrimSuffix(name, ":latest")
}

func (s models) counts() (fits, installed, all int) {
	for _, r := range s.rows {
		if r.candidate != nil {
			fits++
		}
		if r.installed {
			installed++
		}
	}
	return fits, installed, len(s.rows)
}

// apply recomputes the visible rows from the filter, query and sort.
func (s *models) apply() {
	selected := s.selectedID()
	q := strings.ToLower(strings.TrimSpace(s.query.Value()))
	s.visible = make([]int, 0, len(s.rows))
	for i, r := range s.rows {
		switch {
		case s.filter == filterFits && r.candidate == nil:
			continue
		case s.filter == filterInstalled && !r.installed:
			continue
		case q != "" && !strings.Contains(r.search, q):
			continue
		}
		s.visible = append(s.visible, i)
	}
	slices.SortStableFunc(s.visible, func(a, b int) int { return s.compare(s.rows[a], s.rows[b]) })
	s.cursor = 0
	for i, idx := range s.visible {
		if s.rows[idx].id == selected {
			s.cursor = i
		}
	}
	s.offset = 0
	s.ensureVisible()
	s.refresh()
}

func (s models) compare(a, b modelRow) int {
	if (a.candidate == nil) != (b.candidate == nil) {
		if a.candidate != nil {
			return -1
		}
		return 1
	}
	if a.candidate == nil {
		return cmp.Compare(a.rank, b.rank)
	}
	ca, cb := a.candidate.Best, b.candidate.Best
	switch s.sort {
	case sortSessions:
		return cmp.Or(cmp.Compare(cb.Coverage, ca.Coverage), cmp.Compare(a.rank, b.rank))
	case sortSpeed:
		return cmp.Or(cmp.Compare(cb.TokensPerSecond, ca.TokensPerSecond), cmp.Compare(a.rank, b.rank))
	case sortMemory:
		return cmp.Or(cmp.Compare(ca.MemoryBytes, cb.MemoryBytes), cmp.Compare(a.rank, b.rank))
	case sortName:
		return cmp.Compare(strings.ToLower(a.name), strings.ToLower(b.name))
	default:
		return cmp.Compare(a.rank, b.rank)
	}
}

func (s models) selected() (modelRow, bool) {
	if len(s.visible) == 0 {
		return modelRow{}, false
	}
	return s.rows[s.visible[s.cursor]], true
}

func (s models) selectedID() string {
	r, _ := s.selected()
	return r.id
}

// listRows is how many model rows fit above the detail panel.
func (s models) listRows() int {
	if s.height <= 0 {
		return 1
	}
	return min(max(len(s.visible), 1), max(3, (s.height-4)*2/5))
}

func (s models) panelHeight() int {
	return max(s.height-s.listRows()-4, 3)
}

func (s *models) ensureVisible() {
	rows := s.listRows()
	if s.cursor < s.offset {
		s.offset = s.cursor
	}
	if s.cursor >= s.offset+rows {
		s.offset = s.cursor - rows + 1
	}
}

// refresh re-renders the detail panel and the full view for the selection.
func (s *models) refresh() {
	s.panel.SetWidth(s.width)
	s.panel.SetHeight(s.panelHeight())
	s.full.SetWidth(s.width)
	s.full.SetHeight(max(s.height, 3))
	r, ok := s.selected()
	if !ok {
		s.panel.SetContent(s.env.theme.Muted.Render("No models match."))
		return
	}
	s.panel.SetContent(s.detail(r, s.width, false))
	s.panel.GotoTop()
	switch s.mode {
	case modeFull:
		s.full.SetContent(s.detail(r, s.width, true))
	case modeCompare:
		s.full.SetContent(s.compareView(s.width))
	default:
	}
}

func (s *models) move(delta int) {
	if len(s.visible) == 0 {
		return
	}
	next := min(max(s.cursor+delta, 0), len(s.visible)-1)
	if next != s.cursor {
		s.cursor = next
		s.ensureVisible()
		s.refresh()
	}
}

func (s models) capturesKeys() bool { return s.typing || s.mode != modeList }

func (s models) update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case sizeMsg:
		s.width, s.height = msg.width, msg.height
		s.query.SetWidth(max(s.width/3, 12))
		s.ensureVisible()
		s.refresh()
	case compareTopMsg:
		s.marked = slices.Clone(msg.ids)
		s.filter = filterFits
		s.apply()
		s.mode = modeCompare
		s.refresh()
	case selectModelMsg:
		s.filter = filterAll
		s.query.SetValue("")
		s.apply()
		for i, idx := range s.visible {
			if s.rows[idx].id == msg.id {
				s.cursor = i
			}
		}
		s.ensureVisible()
		s.refresh()
	case tea.KeyPressMsg:
		return s.key(msg)
	case tea.MouseWheelMsg:
		if s.mode != modeList {
			var cmd tea.Cmd
			s.full, cmd = s.full.Update(msg)
			return s, cmd
		}
		switch msg.Mouse().Button {
		case tea.MouseWheelUp:
			s.move(-1)
		case tea.MouseWheelDown:
			s.move(1)
		default:
		}
	}
	return s, nil
}

func (s models) key(msg tea.KeyPressMsg) (screen, tea.Cmd) {
	k := s.env.keys
	if s.typing {
		switch msg.String() {
		case "esc":
			s.typing = false
			s.query.Blur()
			s.query.SetValue("")
			s.apply()
		case "enter", "up", "down":
			s.typing = false
			s.query.Blur()
			if msg.String() != "enter" {
				return s.key(msg)
			}
		default:
			var cmd tea.Cmd
			s.query, cmd = s.query.Update(msg)
			s.apply()
			return s, cmd
		}
		return s, nil
	}
	if s.mode != modeList {
		switch {
		case key.Matches(msg, k.Back), msg.String() == "q", key.Matches(msg, k.Open) && s.mode == modeFull:
			s.mode = modeList
		case msg.String() == "c" && s.mode == modeCompare:
			s.mode = modeList
		case key.Matches(msg, k.Top):
			s.full.GotoTop()
		case key.Matches(msg, k.Bottom):
			s.full.GotoBottom()
		default:
			var cmd tea.Cmd
			s.full, cmd = s.full.Update(msg)
			return s, cmd
		}
		return s, nil
	}
	switch {
	case key.Matches(msg, k.Up):
		s.move(-1)
	case key.Matches(msg, k.Down):
		s.move(1)
	case key.Matches(msg, k.PageUp):
		s.move(-s.listRows())
	case key.Matches(msg, k.PageDown):
		s.move(s.listRows())
	case key.Matches(msg, k.Top):
		s.move(-len(s.visible))
	case key.Matches(msg, k.Bottom):
		s.move(len(s.visible))
	case key.Matches(msg, k.ScrollUp):
		s.panel.ScrollUp(3)
	case key.Matches(msg, k.ScrollDown):
		s.panel.ScrollDown(3)
	case key.Matches(msg, k.Filter):
		s.typing = true
		return s, s.query.Focus()
	case key.Matches(msg, k.Back):
		if s.query.Value() != "" {
			s.query.SetValue("")
			s.apply()
		}
	case msg.String() == "f":
		s.filter = (s.filter + 1) % 3
		s.apply()
	case msg.String() == "s":
		s.sort = (s.sort + 1) % modelSort(len(sortNames))
		s.apply()
	case msg.String() == "space":
		s.toggleMark()
	case msg.String() == "t" && s.env.try:
		if r, ok := s.selected(); ok && r.candidate != nil {
			return s, try(r.id)
		}
	case msg.String() == "b" && s.env.change:
		if r, ok := s.selected(); ok && r.candidate != nil && r.candidate.Installed != nil {
			return s, func() tea.Msg { return benchmarkMsg{model: r.id} }
		}
	case msg.String() == "p" && s.env.change:
		if r, ok := s.selected(); ok && r.candidate != nil && r.candidate.Installed == nil {
			return s, func() tea.Msg { return pullMsg{model: r.id} }
		}
	case msg.String() == "c":
		s.mode = modeCompare
		s.full.GotoTop()
		s.refresh()
	case key.Matches(msg, k.Open):
		if _, ok := s.selected(); ok {
			s.mode = modeFull
			s.full.GotoTop()
			s.refresh()
		}
	}
	return s, nil
}

func (s *models) toggleMark() {
	r, ok := s.selected()
	if !ok || r.candidate == nil {
		return
	}
	if i := slices.Index(s.marked, r.id); i >= 0 {
		s.marked = slices.Delete(slices.Clone(s.marked), i, i+1)
	} else if len(s.marked) < 4 {
		s.marked = append(slices.Clone(s.marked), r.id)
	}
}

func (s models) hints() []hint {
	switch {
	case s.typing:
		return []hint{{"type", "to filter"}, {"enter", "keep"}, {"esc", "clear"}}
	case s.mode == modeFull:
		return []hint{{"↑↓", "scroll"}, {"esc", "back to list"}}
	case s.mode == modeCompare:
		return []hint{{"↑↓", "scroll"}, {"esc", "back to list"}}
	}
	compare := "compare"
	if len(s.marked) >= 2 {
		compare = "compare " + strconv.Itoa(len(s.marked))
	}
	hints := []hint{{"↑↓", "model"}, {"enter", "details"}}
	if s.env.try {
		hints = append(hints, hint{"t", "try"})
	}
	if r, ok := s.selected(); ok && s.env.change && r.candidate != nil && r.candidate.Installed == nil {
		hints = append(hints, hint{"p", "download"})
	}
	if r, ok := s.selected(); ok && s.env.change && r.candidate != nil && r.candidate.Installed != nil && r.candidate.Best.Curve.Source != match.SourceMeasured {
		hints = append(hints, hint{"b", "measure"})
	}
	return append(hints, hint{"c", compare}, hint{"f", "filter"}, hint{"s", "sort"}, hint{"/", "search"})
}

// help lists every key the screen understands, for the help overlay.
func (s models) help() []hint {
	h := []hint{
		{"↑↓", "move between models"},
		{"enter", "full details and every way to run it"},
		{"space", "mark for comparison, up to four"},
		{"c", "compare marked models, or the three best"},
		{"f", "show models that fit, are installed, or all"},
		{"s", "sort by fit, sessions, speed, memory or name"},
		{"/", "search by name, family or lab"},
		{"J K", "scroll the details"},
		{"esc", "clear the search or go back"},
	}
	if s.env.change {
		h = slices.Insert(h, 2, hint{"p", "download a model that fits and set it up, ready to try"},
			hint{"b", "measure a downloaded model's real speed on this machine"})
	}
	if s.env.try {
		h = slices.Insert(h, 2, hint{"t", "try it for one session, changing no settings"})
	}
	return h
}

func (s models) view(width, _ int) string {
	if s.mode != modeList {
		return s.full.View()
	}
	t := s.env.theme
	var b strings.Builder
	b.WriteString(s.chips(width) + "\n\n")
	cols := s.columns(width)
	b.WriteString(cols.header(t) + "\n")
	rows := s.listRows()
	for i := s.offset; i < min(s.offset+rows, len(s.visible)); i++ {
		b.WriteString(cols.row(s, s.rows[s.visible[i]], i == s.cursor) + "\n")
	}
	if len(s.visible) == 0 {
		b.WriteString(t.Muted.Render("  No models match. Press f to show more, or esc to clear the search.") + "\n")
	}
	for i := min(len(s.visible), rows); i < rows; i++ {
		b.WriteString("\n")
	}
	b.WriteString(t.Muted.Render(strings.Repeat("─", width)) + "\n")
	b.WriteString(s.panel.View())
	return b.String()
}

func (s models) chips(width int) string {
	t := s.env.theme
	fits, installed, all := s.counts()
	chip := func(label string, n int, on bool) string {
		text := label + " " + strconv.Itoa(n)
		if on {
			return t.Accent.Render("● ") + t.Accent.Bold(true).Render(text)
		}
		return t.Muted.Render("○ ") + t.Label.Render(text)
	}
	left := chip("Fits", fits, s.filter == filterFits) + "    " +
		chip("Installed", installed, s.filter == filterInstalled) + "    " +
		chip("All", all, s.filter == filterAll)
	right := t.Muted.Render("sort ") + t.Value.Render(sortNames[s.sort])
	if s.typing || s.query.Value() != "" {
		right = s.query.View() + "   " + right
	}
	if len(s.marked) > 0 {
		right = t.Accent.Render(strconv.Itoa(len(s.marked))+" marked") + "   " + right
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 2 {
		return left
	}
	return left + strings.Repeat(" ", gap) + right
}

// speedColumn fits a speed and its pace, "  32 t/s  2.9× slower".
const speedColumn = 8 + 2 + 11

// modelColumns lays out the list for a given width.
type modelColumns struct {
	name, bar, mem int
}

func (s models) columns(width int) modelColumns {
	c := modelColumns{bar: 12, mem: 10}
	if width < 90 {
		c.bar, c.mem = 8, 0
	}
	fixed := 2 + 2 + (c.bar + 5) + 2 + speedColumn + 2 + 4 + 2 + 8
	if c.mem > 0 {
		fixed += c.mem + 1
	}
	c.name = max(width-fixed, 12)
	return c
}

func (c modelColumns) header(t theme.Theme) string {
	return t.Label.Render("  " + padRight("MODEL", c.name) + "  " + padRight("YOUR SESSIONS", c.bar+5) + "  " +
		padRight("   SPEED", speedColumn) + "  " + padRight("TIER", 4) + "  MEMORY")
}

func (c modelColumns) row(s models, r modelRow, selected bool) string {
	th := s.env.theme
	marker := "  "
	if selected {
		marker = th.Accent.Render("▌ ")
	}
	name := r.name
	if slices.Contains(s.marked, r.id) {
		name = "● " + name
	}
	// The installed mark is status, not part of the name: a long name is
	// cut short before it, never through it.
	mark := ""
	if r.installed {
		mark = " ✓"
	}
	nameStyle := th.Value
	switch {
	case selected:
		nameStyle = th.Accent.Bold(true)
	case r.candidate == nil:
		nameStyle = th.Muted
	}
	cell := ansi.Truncate(nameStyle.Render(name), c.name-lipgloss.Width(mark), "…") + nameStyle.Render(mark)
	out := marker + padRight(cell, c.name) + "  "
	switch {
	case r.candidate != nil:
		o := r.candidate.Best
		style := coverageStyle(th, o.Coverage)
		out += bar(th, o.Coverage, c.bar, style.Render) + " " + style.Render(padLeft(format.Percent(o.Coverage), 4)) + "  " +
			th.Value.Render(padLeft(strconv.Itoa(int(o.TokensPerSecond)), 4)) + th.Muted.Render(" t/s") + "  " +
			padRight(report.PaceLabel(r.candidate.Pace, th), 11) + "  " +
			th.Value.Render(padRight(string(r.candidate.Model.Tier), 4)) + "  "
		frac := 0.0
		if b := s.env.assessment.Match.Budget.Bytes; b > 0 {
			frac = float64(o.MemoryBytes) / float64(b)
		}
		if c.mem > 0 {
			out += bar(th, frac, c.mem, th.Label.Render) + " "
		}
		out += th.Value.Render(padLeft(format.Bytes(uint64(o.MemoryBytes)), 8))
	case r.tooLarge != nil:
		out += th.Muted.Render("too large · needs " + format.Bytes(uint64(r.tooLarge.NeedsBytes)))
	default:
		out += th.Muted.Render("not in the catalogue · " + format.Bytes(r.unknown.SizeBytes))
	}
	return out
}
