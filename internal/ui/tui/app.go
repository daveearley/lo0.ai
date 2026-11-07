// Package tui is lo0's interactive terminal interface: a workbench for
// deciding whether, and how, to move coding-agent work to a local model.
package tui

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/scan"
	"lo0.ai/lo0/internal/ui/progress"
	"lo0.ai/lo0/internal/ui/report"
	"lo0.ai/lo0/internal/ui/theme"
)

// Assessor runs the assessment the interface shows.
type Assessor interface {
	Assess(ctx context.Context, observe scan.Observer) (app.Assessment, error)
}

// Options configure the interface.
type Options struct {
	Service        Assessor
	DarkBackground bool
	// Home is the user's home directory, used to shorten paths.
	Home    string
	Version string
	// Save writes a file in the current directory, failing with an error
	// wrapping fs.ErrExist if it is already there. Now dates the file.
	// Without both, export is not offered.
	Save func(name string, data []byte) error
	Now  func() time.Time
	// Actions plans and applies changes. Session builds the command that
	// runs a try's harness in this terminal; without it, and Now, try is
	// not offered, and without Actions nothing can be changed.
	Actions Actions
	Session func(migrate.Session) tea.ExecCommand
}

type state int

const (
	stateLoading state = iota
	stateReady
	stateFailed
)

// Layout.
const (
	sidebarWidth = 18
	// wideWidth is where the sidebar appears; below it the screens are a
	// line of tabs under the top bar.
	wideWidth  = 100
	sideMargin = 2
	minWidth   = 60
	minHeight  = 16
)

// Model is the root Bubble Tea model.
type Model struct {
	opts   Options
	theme  theme.Theme
	keys   keyMap
	ctx    context.Context
	cancel context.CancelFunc

	// run identifies the current assessment, so messages from one that a
	// rescan cancelled are ignored.
	run        int
	state      state
	progress   progress.Model
	assessment app.Assessment
	err        error

	active screenID
	// screens is indexed by screenID. It is an array, not a map, so that
	// copies of the model never share screen state.
	screens  [len(screenNames)]screen
	palette  palette
	showHelp bool
	// exporting is true while the export confirmation is open; status is
	// the outcome of the last export, shown until the next key.
	exporting bool
	status    string
	// try is the try in progress, if any.
	try flow
	// quitting is true once ctrl+c has stopped a running plan; lo0 quits
	// when the plan returns.
	quitting bool
	// inventory is what is installed, read when the assessment arrives.
	inventory    app.Inventory
	inventoryErr error
	harnessViews []app.HarnessView

	width, height int
}

// New returns the interface model.
func New(opts Options) Model {
	m := Model{opts: opts, theme: theme.New(opts.DarkBackground), keys: defaultKeys(), width: 100, height: 30}
	m.reset()
	return m
}

func (m *Model) reset() {
	m.run++
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.state = stateLoading
	m.err = nil
	m.progress = progress.New(m.theme, m.cancel)
}

// Run starts the interface and blocks until the user quits.
func Run(opts Options, programOpts ...tea.ProgramOption) error {
	_, err := tea.NewProgram(New(opts), programOpts...).Run()
	return err
}

type eventMsg struct {
	run   int
	event scan.Event
	next  tea.Cmd
}

type assessedMsg struct {
	run        int
	assessment app.Assessment
	err        error

	inventory    app.Inventory
	inventoryErr error
	harnesses    []app.HarnessView
}

// start runs the assessment in the background. Events come back through
// channels drained one message at a time by listen.
func (m Model) start() tea.Cmd {
	events := make(chan scan.Event, 64)
	done := make(chan assessedMsg, 1)
	ctx, run := m.ctx, m.run
	go func() {
		a, err := m.opts.Service.Assess(ctx, func(e scan.Event) {
			// Counter updates are dropped when the interface is behind: the
			// next one supersedes them, and blocking here would stall every
			// scan worker. Stage changes always get through.
			if e.Total > 0 && e.Status == scan.StatusRunning {
				select {
				case events <- e:
				default:
				}
				return
			}
			select {
			case events <- e:
			case <-ctx.Done():
			}
		})
		msg := assessedMsg{run: run, assessment: a, err: err}
		// Reading the inventory runs each harness's --version, which can
		// take seconds, so it happens here rather than in Update.
		if err == nil && m.opts.Actions != nil {
			msg.inventory, msg.inventoryErr = m.opts.Actions.Inventory(a)
			msg.harnesses = m.opts.Actions.Harnesses(ctx, a)
		}
		close(events)
		done <- msg
	}()
	return listen(run, events, done)
}

func listen(run int, events <-chan scan.Event, done <-chan assessedMsg) tea.Cmd {
	return func() tea.Msg {
		if e, ok := <-events; ok {
			return eventMsg{run: run, event: e, next: listen(run, events, done)}
		}
		return <-done
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.progress.Init(), m.start(), tea.RequestBackgroundColor)
}

func (m Model) env() env {
	return env{
		theme: m.theme, keys: m.keys, assessment: m.assessment, home: m.opts.Home,
		export: m.canExport(), try: m.canTry(), change: m.opts.Actions != nil,
		inventory: m.inventory, inventoryErr: m.inventoryErr, harnesses: m.harnessViews,
	}
}

// build creates every screen from the assessment.
func (m *Model) build() {
	e := m.env()
	m.screens[screenHome] = newHome(e)
	m.screens[screenModels] = newModels(e)
	m.screens[screenSessions] = newClaude(e)
	m.screens[screenAgents] = newHarnesses(e)
	m.screens[screenMachine] = newInstalled(e)
	m.palette = newPalette(e)
	m.resize()
}

// resize passes the content area's size to every screen.
func (m *Model) resize() {
	w, h := m.contentSize()
	for id, s := range m.screens {
		if s == nil {
			continue
		}
		next, _ := s.update(sizeMsg{width: w, height: h})
		m.screens[id] = next
	}
	m.palette.setWidth(min(w, 72))
}

// sizeMsg tells a screen how much room it has.
type sizeMsg struct{ width, height int }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if next, cmd, ok := m.updateTry(msg); ok {
		return next, cmd
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.state == stateReady {
			m.resize()
		}
		return m, nil
	case tea.BackgroundColorMsg:
		return m.restyle(msg.IsDark())
	case eventMsg:
		if msg.run != m.run {
			return m, msg.next
		}
		next, _ := m.progress.Update(progress.EventMsg(msg.event))
		m.progress = next.(progress.Model)
		return m, msg.next
	case assessedMsg:
		if msg.run != m.run {
			return m, nil
		}
		if msg.err != nil {
			if m.ctx.Err() != nil {
				return m, tea.Quit
			}
			m.state, m.err = stateFailed, msg.err
			return m, nil
		}
		m.state, m.assessment = stateReady, msg.assessment
		m.inventory, m.inventoryErr, m.harnessViews = msg.inventory, msg.inventoryErr, msg.harnesses
		m.build()
		return m, nil
	case exportMsg:
		m.palette.close()
		m.exporting = m.canExport()
		return m, nil
	case savedMsg:
		if msg.err != nil {
			m.status = m.theme.Bad.Render("Could not save the report: " + msg.err.Error())
		} else {
			m.status = m.theme.Good.Render("Saved " + msg.name)
		}
		return m, nil
	case navigateMsg:
		m.active = msg.to
		m.palette.close()
		if msg.then != nil {
			return m.toScreen(msg.then)
		}
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tea.MouseClickMsg:
		return m.click(msg.Mouse())
	}
	if m.state == stateLoading {
		next, cmd := m.progress.Update(msg)
		m.progress = next.(progress.Model)
		return m, cmd
	}
	return m.toScreen(msg)
}

func (m Model) restyle(dark bool) (tea.Model, tea.Cmd) {
	if dark == m.opts.DarkBackground {
		return m, nil
	}
	m.opts.DarkBackground = dark
	m.theme = theme.New(dark)
	m.progress = m.progress.WithTheme(m.theme)
	if m.state != stateReady {
		return m, nil
	}
	m.build()
	return m, nil
}

func (m Model) toScreen(msg tea.Msg) (tea.Model, tea.Cmd) {
	s := m.screens[m.active]
	if s == nil {
		return m, nil
	}
	next, cmd := s.update(msg)
	m.screens[m.active] = next
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		m.cancel()
		if m.try.cancel != nil {
			m.try.cancel()
		}
		// A plan stops between steps and records each change as it goes,
		// so lo0 quits once the step under way has finished.
		if m.try.stage == stageRunning {
			m.try.stopped, m.quitting = true, true
			return m, nil
		}
		return m, tea.Quit
	}
	if m.state != stateReady {
		switch {
		case key.Matches(msg, m.keys.Quit):
			m.cancel()
			return m, tea.Quit
		case m.state == stateFailed && (msg.String() == "r" || key.Matches(msg, m.keys.Rescan)):
			m.reset()
			return m, tea.Batch(m.progress.Init(), m.start())
		}
		return m, nil
	}
	m.status = ""
	if m.try.active {
		return m.handleTryKey(msg)
	}
	if m.exporting {
		return m.handleExportKey(msg)
	}
	if m.palette.open {
		var cmd tea.Cmd
		m.palette, cmd = m.palette.update(msg)
		return m, cmd
	}
	if m.showHelp {
		if key.Matches(msg, m.keys.Help, m.keys.Back, m.keys.Quit) {
			m.showHelp = false
		}
		return m, nil
	}
	if s := m.screens[m.active]; s != nil && s.capturesKeys() {
		return m.toScreen(msg)
	}
	switch {
	case key.Matches(msg, m.keys.Quit):
		m.cancel()
		return m, tea.Quit
	case key.Matches(msg, m.keys.Palette):
		return m, m.palette.show()
	case key.Matches(msg, m.keys.Help):
		m.showHelp = true
		return m, nil
	case key.Matches(msg, m.keys.Export) && m.canExport():
		m.exporting = true
		return m, nil
	case key.Matches(msg, m.keys.NextScreen):
		m.active = screenOrder[(indexOf(m.active)+1)%len(screenOrder)]
		return m, nil
	case key.Matches(msg, m.keys.PrevScreen):
		m.active = screenOrder[(indexOf(m.active)+len(screenOrder)-1)%len(screenOrder)]
		return m, nil
	case key.Matches(msg, m.keys.Back) && m.active != screenHome && !m.nested():
		m.active = screenHome
		return m, nil
	case key.Matches(msg, m.keys.Rescan):
		m.cancel()
		m.reset()
		return m, tea.Batch(m.progress.Init(), m.start())
	}
	if r := msg.String(); len(r) == 1 && r[0] >= '1' && int(r[0]-'1') < len(screenOrder) {
		m.active = screenOrder[r[0]-'1']
		return m, nil
	}
	return m.toScreen(msg)
}

// nested reports whether the active screen has a view open that esc closes
// first.
func (m Model) nested() bool {
	n, ok := m.screens[m.active].(nester)
	return ok && n.nested()
}

// click switches screens from the sidebar or the tabs, and passes a click in
// the content area to the screen, in its own coordinates.
func (m Model) click(c tea.Mouse) (tea.Model, tea.Cmd) {
	if m.state != stateReady || c.Button != tea.MouseLeft || m.try.active || m.exporting || m.palette.open || m.showHelp {
		return m, nil
	}
	x, y := c.X-sideMargin, c.Y
	if m.wide() {
		if i := y - 2; x >= 0 && x < sidebarWidth && i >= 0 && i < len(screenOrder) {
			m.active = screenOrder[i]
			return m, nil
		}
		x -= sidebarWidth + 2
	} else {
		if y == 1 {
			_, spans := m.tabs()
			for i, sp := range spans {
				if x >= sp[0] && x < sp[1] {
					m.active = screenOrder[i]
				}
			}
			return m, nil
		}
		y--
	}
	// Content starts below the top bar, a blank line, the title and
	// another blank line.
	if x < 0 || y < 4 {
		return m, nil
	}
	return m.toScreen(clickMsg{x: x, y: y - 4})
}

func indexOf(id screenID) int {
	for i, s := range screenOrder {
		if s == id {
			return i
		}
	}
	return 0
}

func (m Model) wide() bool { return m.width >= wideWidth }

// contentSize is the area screens draw in: the window minus margins, the
// sidebar or the tabs, the top bar, the title and the footer.
func (m Model) contentSize() (int, int) {
	w, h := m.width-2*sideMargin, m.height-6
	if m.wide() {
		w -= sidebarWidth + 2
	} else {
		h--
	}
	return max(w, 20), max(h, 4)
}

// View implements tea.Model.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "lo0"
	return v
}

func (m Model) render() string {
	if m.width < minWidth || m.height < minHeight {
		return m.theme.Muted.Render("lo0 needs a window of at least " + format.Int(minWidth) + "×" + format.Int(minHeight) + ".")
	}
	var body string
	switch m.state {
	case stateLoading:
		body = m.loadingView()
	case stateFailed:
		body = m.failedView()
	default:
		body = m.readyView()
	}
	lines := strings.Split(body, "\n")
	for len(lines) < m.height-1 {
		lines = append(lines, "")
	}
	lines = append(lines[:m.height-1], m.footerView())
	pad := strings.Repeat(" ", sideMargin)
	for i, l := range lines {
		lines[i] = pad + ansi.Truncate(l, m.width-2*sideMargin, "…")
	}
	return strings.Join(lines, "\n")
}

func (m Model) topBar() string {
	w := m.width - 2*sideMargin
	left := m.theme.Title.Render("lo0")
	right, short := "", ""
	if m.state == stateReady {
		parts := report.MachineChips(m.assessment.Scan.Machine, m.assessment.Match.Budget)
		for _, rt := range m.assessment.Scan.Runtimes {
			if rt.Running {
				parts = append(parts, rt.Name+" running")
				break
			}
		}
		right = m.theme.Muted.Render(strings.Join(parts, " · "))
		// Keep the simulated notice when the rest is cut.
		if m.assessment.Scan.Machine.Simulated {
			short = m.theme.Warn.Render(parts[0])
			right = short + m.theme.Muted.Render(" · "+strings.Join(parts[1:], " · "))
		}
	}
	fits := []string{right}
	if short != "" {
		fits = append(fits, short)
	}
	for _, r := range fits {
		if gap := w - lipgloss.Width(left) - lipgloss.Width(r); gap >= 2 {
			return left + strings.Repeat(" ", gap) + r
		}
	}
	return left
}

func (m Model) readyView() string {
	cw, ch := m.contentSize()
	question := screenQuestions[m.active]
	if t, ok := m.screens[m.active].(titled); ok {
		question = t.title()
	}
	if m.try.active {
		question = m.tryTitle()
	}
	title := m.theme.Heading.Render(question)
	content := ""
	switch {
	case m.try.active:
		content = m.tryView(cw, ch)
	case m.exporting:
		content = m.exportView(cw)
	case m.showHelp:
		content = m.helpView(cw)
	case m.palette.open:
		content = m.palette.view(ch)
	default:
		content = m.screens[m.active].view(cw, ch)
	}
	main := fitLines(title+"\n\n"+content, cw, ch+2)
	if m.wide() {
		return m.topBar() + "\n\n" + lipgloss.JoinHorizontal(lipgloss.Top, m.sidebar(ch+2), "  ", main)
	}
	tabs, _ := m.tabs()
	return m.topBar() + "\n" + tabs + "\n\n" + main
}

// tabs are the screens on one line, for windows too narrow for the sidebar,
// with the columns each one spans.
func (m Model) tabs() (string, [][2]int) {
	var b strings.Builder
	spans := make([][2]int, 0, len(screenOrder))
	for i, id := range screenOrder {
		if i > 0 {
			b.WriteString("  ")
		}
		start := lipgloss.Width(b.String())
		num := m.theme.Muted.Render(format.Int(i+1) + " ")
		if id == m.active {
			b.WriteString(m.theme.Accent.Render("▌ ") + num + m.theme.Accent.Bold(true).Render(screenNames[id]))
		} else {
			b.WriteString("  " + num + m.theme.Label.Render(screenNames[id]))
		}
		spans = append(spans, [2]int{start, lipgloss.Width(b.String())})
	}
	return b.String(), spans
}

func (m Model) sidebar(height int) string {
	lines := make([]string, 0, height)
	for i, id := range screenOrder {
		label := screenNames[id]
		num := m.theme.Muted.Render(format.Int(i+1) + " ")
		if id == m.active {
			lines = append(lines, m.theme.Accent.Render("▌ ")+num+m.theme.Accent.Bold(true).Render(label))
		} else {
			lines = append(lines, "  "+num+m.theme.Label.Render(label))
		}
	}
	return lipgloss.NewStyle().Width(sidebarWidth).Render(fitLines(strings.Join(lines, "\n"), sidebarWidth, height))
}

func (m Model) loadingView() string {
	text := m.topBar() + "\n\n" +
		m.theme.Heading.Render("Reading your machine and your Claude Code and Codex history") + "\n" +
		m.theme.Muted.Render("Everything stays on this machine. Nothing is changed.") + "\n\n" +
		m.progress.View().Content
	return text
}

func (m Model) failedView() string {
	return m.topBar() + "\n\n" + m.theme.Bad.Render("The scan failed.") + "\n\n" +
		m.theme.Value.Render(m.err.Error()) + "\n\n" + m.theme.Muted.Render("Press r to try again or q to quit.")
}

func (m Model) footerView() string {
	var hints []hint
	switch {
	case m.state == stateLoading:
		hints = []hint{{"q", "stop"}}
	case m.state == stateFailed:
		hints = []hint{{"r", "try again"}, {"q", "quit"}}
	case m.try.active:
		hints = m.tryHints()
	case m.exporting:
		hints = []hint{{"enter", "save"}, {"esc", "cancel"}}
	case m.palette.open:
		hints = []hint{{"type", "to search"}, {"↑↓", "choose"}, {"enter", "go"}, {"esc", "close"}}
	case m.showHelp:
		hints = []hint{{"esc", "close"}}
	default:
		hints = append(m.screens[m.active].hints(), hint{"ctrl+k", "commands"}, hint{"?", "keys"})
	}
	parts := make([]string, 0, len(hints)+1)
	if m.status != "" {
		parts = append(parts, m.status)
	}
	for _, h := range hints {
		parts = append(parts, m.theme.Value.Render(h.key)+" "+m.theme.Muted.Render(h.desc))
	}
	return strings.Join(parts, "   ")
}

func (m Model) helpView(width int) string {
	global := []hint{
		fromBinding(m.keys.NextScreen), fromBinding(m.keys.PrevScreen),
		{"1–5", strings.Join(screenNames[:], ", ")},
		{"esc", "back, then Home"},
		fromBinding(m.keys.Palette), fromBinding(m.keys.Rescan), fromBinding(m.keys.Help), fromBinding(m.keys.Quit),
	}
	if m.canExport() {
		global = append(global[:len(global)-1], fromBinding(m.keys.Export), global[len(global)-1])
	}
	var b strings.Builder
	b.WriteString(m.theme.Label.Render(strings.ToUpper(screenNames[m.active])) + "\n")
	screenHints := m.screens[m.active].hints()
	if h, ok := m.screens[m.active].(helper); ok {
		screenHints = h.help()
	}
	// A key that works everywhere is listed once, under EVERYWHERE, even
	// when the screen's own footer shows it too.
	everywhere := map[string]bool{}
	for _, h := range global {
		everywhere[h.key] = true
	}
	for _, h := range screenHints {
		if !everywhere[h.key] {
			b.WriteString(m.helpLine(h) + "\n")
		}
	}
	b.WriteString("\n" + m.theme.Label.Render("EVERYWHERE") + "\n")
	for _, h := range global {
		b.WriteString(m.helpLine(h) + "\n")
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(b.String())
}

func (m Model) helpLine(h hint) string {
	return "  " + m.theme.Value.Render(padLeft(h.key, 10)) + "   " + m.theme.Muted.Render(h.desc)
}

// fitLines cuts s to height lines and width columns, padding short output so
// the layout below it stays put.
func fitLines(s string, width, height int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, width, "…")
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func padLeft(s string, width int) string {
	return strings.Repeat(" ", max(width-lipgloss.Width(s), 0)) + s
}

func padRight(s string, width int) string {
	return s + strings.Repeat(" ", max(width-lipgloss.Width(s), 0))
}
