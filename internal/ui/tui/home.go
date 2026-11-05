package tui

import (
	"math"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/ui/draw"
	"lo0.ai/lo0/internal/ui/report"
)

const (
	// daysPerMonth is the mean length of a Gregorian month.
	daysPerMonth = 30.44
	columnGap    = 4
	// stackedWidth is the content width below which Home stacks its panels
	// and turns the fact columns into labelled rows.
	stackedWidth = 84
)

// home is the Home screen: the user's local setup at a glance and what to
// do next, each step one key away.
type home struct {
	env    env
	next   app.Overview
	steps  []step
	cursor int
}

// step is a next step the interface can carry out.
type step struct {
	app.NextStep
	run tea.Cmd
}

func newHome(e env) screen {
	h := home{env: e, next: app.Next(e.assessment, e.inventory, e.harnesses)}
	for _, s := range h.next.Steps {
		if run := h.command(s); run != nil {
			h.steps = append(h.steps, step{NextStep: s, run: run})
		}
	}
	return h
}

// command is what carrying out s does here, or nil when this interface
// cannot, such as a try without a way to start the harness.
func (h home) command(s app.NextStep) tea.Cmd {
	msg := func(m tea.Msg) tea.Cmd { return func() tea.Msg { return m } }
	switch s.Kind {
	case app.StepTry:
		if h.env.try {
			return msg(tryInMsg{harness: s.Harness})
		}
	case app.StepKeep:
		if h.env.change {
			return msg(keepInMsg{harness: s.Harness})
		}
	case app.StepDownload:
		if h.env.change {
			return msg(pullMsg{model: s.Model})
		}
	case app.StepInstall:
		if h.env.change {
			return msg(installMsg{harness: s.Harness})
		}
	case app.StepRemove:
		if h.env.change {
			return msg(removeMsg{runtime: s.Runtime, model: s.Remove})
		}
	case app.StepCompare:
		if best, ok := h.env.assessment.Match.Best(); ok {
			return navigate(screenModels, compareTopMsg{ids: []string{best.Model.ID, s.Model}})
		}
	case app.StepBrowse:
		return navigate(screenModels, nil)
	}
	return nil
}

func (h home) update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, h.env.keys.Up):
			h.cursor = max(h.cursor-1, 0)
		case key.Matches(msg, h.env.keys.Down):
			h.cursor = min(h.cursor+1, len(h.steps)-1)
		case key.Matches(msg, h.env.keys.Open) && h.cursor < len(h.steps):
			return h, h.steps[h.cursor].run
		}
	case tea.MouseWheelMsg:
		switch msg.Mouse().Button {
		case tea.MouseWheelUp:
			h.cursor = max(h.cursor-1, 0)
		case tea.MouseWheelDown:
			h.cursor = min(h.cursor+1, len(h.steps)-1)
		default:
		}
	}
	return h, nil
}

func (h home) hints() []hint {
	return []hint{{"↑↓", "choose"}, {"enter", "go"}, {"tab", "next screen"}}
}

func (home) capturesKeys() bool { return false }

func (h home) view(width, height int) string {
	answer := h.answer(width)
	// With the answer and the race drawn, the next step need not repeat
	// the share or the speed in words.
	next := h.nextStep(width, len(answer) > 0 && len(h.race(width)) > 0)
	facts := h.facts(width)

	// Below the answer, two panels side by side when there is room: how
	// long a reply takes, and where each command sends it. Stacked, they
	// are separate blocks so a short window can keep the race alone.
	var race, routes []string
	if width >= stackedWidth {
		// The routes take the width they need and the race the rest, so
		// its bars are as long as the window allows.
		rw := lipgloss.Width("WHERE YOUR COMMANDS GO")
		for _, l := range h.routes() {
			rw = max(rw, lipgloss.Width(l))
		}
		rw = min(rw, width/2)
		pw := width - columnGap - rw
		race = h.panel("A TYPICAL REPLY", h.race(pw), pw)
		routes = h.panel("WHERE YOUR COMMANDS GO", h.routes(), rw)
		if len(race) > 1 && len(routes) > 1 {
			left := lipgloss.NewStyle().Width(pw).Render(strings.Join(race, "\n"))
			right := lipgloss.NewStyle().Width(rw).Render(strings.Join(routes, "\n"))
			race, routes = strings.Split(lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", columnGap), right), "\n"), nil
		}
	} else {
		race = h.panel("A TYPICAL REPLY", h.race(width), width)
		routes = h.panel("WHERE YOUR COMMANDS GO", h.routes(), width)
	}

	// The answer and the next step always show. The rest is dropped on a
	// short window, least useful first: the facts, then the routes, then
	// the race, which Models also draws.
	blocks := [][]string{answer, race, routes, next, facts}
	used := func() int {
		n := 0
		for _, b := range blocks {
			if len(b) > 0 {
				n += len(b) + 1
			}
		}
		return n
	}
	if height > 0 {
		for _, drop := range []int{4, 2, 1} {
			if used() > height {
				blocks[drop] = nil
			}
		}
	}
	var out []string
	for _, b := range blocks {
		if len(b) == 0 {
			continue
		}
		if len(out) > 0 {
			out = append(out, "")
		}
		out = append(out, b...)
	}
	return strings.Join(out, "\n")
}

// answer is the line Home opens on: how much of the user's work runs on this
// machine, as a figure and as a bar split at the best model's window.
func (h home) answer(width int) []string {
	t := h.env.theme
	a := h.env.assessment
	best, ok := a.Match.Best()
	peaks := a.Scan.Usage.Context.Peaks
	if !ok || len(peaks) == 0 {
		return nil
	}
	fit, _ := slices.BinarySearch(peaks, best.Best.MaxContext+1)
	share := best.Best.Coverage
	style := coverageStyle(t, share)
	headline := style.Bold(true).Render(format.Percent(share)) + t.Value.Bold(true).Render(" of your work can run on this machine today")
	cells := min(width, 96)
	filled, track := draw.Bar(share, cells)
	legend := t.Value.Render(format.Plural(fit, "session", "sessions")) + t.Muted.Render(" fit in "+best.Model.Name+"'s "+format.Tokens(best.Best.MaxContext)+" window")
	if rest := len(peaks) - fit; rest > 0 {
		legend += t.Muted.Render(" · ") + t.Value.Render(format.Int(rest)) + t.Muted.Render(" would compact")
	}
	return []string{headline, style.Render(filled) + t.Muted.Render(track), ansi.Truncate(legend, width, "…")}
}

// race is the reply race for the best model, without its label.
func (h home) race(width int) []string {
	a := h.env.assessment
	best, ok := a.Match.Best()
	if !ok {
		return nil
	}
	return report.ReplyRace(best, a.Match.References, h.env.theme, width)
}

// routes draws where each harness command sends its requests, local
// models in the accent, hosted ones muted, and what lo0 last tried.
func (h home) routes() []string {
	t := h.env.theme
	type route struct {
		command, model string
		local          bool
	}
	var rs []route
	for _, v := range h.env.harnesses {
		for _, u := range v.Installation.Uses {
			rs = append(rs, route{u.Command, u.Model, u.Local})
		}
	}
	if len(rs) == 0 {
		return nil
	}
	cmdW := 0
	for _, r := range rs {
		cmdW = max(cmdW, lipgloss.Width(r.command))
	}
	out := make([]string, 0, len(rs)+2)
	for _, r := range rs {
		arrow, model := t.Muted.Render(" ──▶ "), t.Muted.Render(r.model)
		if r.local {
			arrow, model = t.Accent.Render(" ──▶ "), t.Accent.Render(r.model)
		}
		out = append(out, t.Value.Render(padRight(r.command, cmdW))+arrow+model)
	}
	if h.next.Kept == "" {
		out = append(out, t.Muted.Render("no local model kept yet"))
	}
	if !h.next.LastTry.IsZero() {
		out = append(out, t.Muted.Render("last tried ")+t.Value.Render(h.next.LastTryModel)+t.Muted.Render(" on "+h.next.LastTry.Format("2 Jan")))
	}
	return out
}

// panel is a labelled block cut to width.
func (h home) panel(label string, lines []string, width int) []string {
	if len(lines) == 0 {
		return nil
	}
	out := []string{h.env.theme.Label.Render(label)}
	for _, l := range lines {
		out = append(out, ansi.Truncate(l, width, "…"))
	}
	return out
}

// nextStep is the recommendation and the steps that carry it out.
func (h home) nextStep(width int, drawn bool) []string {
	t := h.env.theme
	out := []string{t.Label.Render("BEST NEXT STEP")}
	headline := h.next.Headline
	if drawn && h.next.Status != "" {
		headline = h.next.Status
	}
	for l := range strings.SplitSeq(report.Wrap(headline, min(width, 100)), "\n") {
		out = append(out, t.Value.Render(l))
	}
	out = append(out, "")
	labelW := 0
	for _, s := range h.steps {
		labelW = max(labelW, lipgloss.Width(s.Label))
	}
	labelW = min(labelW, width-4)
	for i, s := range h.steps {
		label := padRight(ansi.Truncate(s.Label, labelW, "…"), labelW)
		line := "  " + t.Value.Render(label)
		if i == h.cursor {
			line = t.Accent.Render("› ") + t.Accent.Bold(true).Render(label)
		}
		// A note is shown whole or not at all.
		if s.Note != "" && lipgloss.Width(line)+4+lipgloss.Width(s.Note) <= width {
			line += "    " + t.Muted.Render(s.Note)
		}
		out = append(out, line)
	}
	return out
}

// facts are the columns under the next step: the machine and the history,
// and where the user works when the routes are not drawn.
func (h home) facts(width int) []string {
	columns := []column{h.thisMachine(), h.yourClaude()}
	if len(h.routes()) == 0 {
		columns = append([]column{h.workingIn()}, columns...)
	}
	if width < stackedWidth {
		out := make([]string, 0, len(columns))
		for _, c := range columns {
			out = append(out, c.row(h, width))
		}
		return out
	}
	cw := (width - 2*columnGap) / 3
	parts := make([]string, 0, 2*len(columns))
	for i, c := range columns {
		if i > 0 {
			parts = append(parts, strings.Repeat(" ", columnGap))
		}
		parts = append(parts, lipgloss.NewStyle().Width(cw).Render(c.render(h, cw)))
	}
	return strings.Split(lipgloss.JoinHorizontal(lipgloss.Top, parts...), "\n")
}

// column is one of the facts under Home's next step.
type column struct {
	label string
	lines func(h home) []string
}

func (c column) render(h home, width int) string {
	lines := c.lines(h)
	out := make([]string, 0, 1+len(lines))
	out = append(out, h.env.theme.Label.Render(c.label))
	for _, l := range lines {
		out = append(out, ansi.Truncate(l, width, "…"))
	}
	return strings.Join(out, "\n")
}

// row is the column as one labelled line, for narrow windows.
func (c column) row(h home, width int) string {
	t := h.env.theme
	const labelW = 15
	return t.Label.Render(padRight(c.label, labelW)) + ansi.Truncate(strings.Join(c.lines(h), t.Muted.Render(" · ")), width-labelW, "…")
}

func (h home) workingIn() column {
	return column{"WORKING IN", func(h home) []string {
		t := h.env.theme
		var out []string
		for _, v := range h.env.harnesses {
			for _, u := range v.Installation.Uses {
				model := t.Muted.Render(u.Model)
				if u.Local {
					model = t.Value.Render(u.Model)
				}
				out = append(out, t.Value.Render(u.Command)+t.Muted.Render(" → ")+model)
			}
		}
		switch {
		case len(h.env.harnesses) == 0:
			// The harnesses were not read, so only the recommendation is
			// known.
			if target := h.env.assessment.Match.Target; target != "" {
				out = append(out, t.Muted.Render("recommended: ")+t.Value.Render(target))
			}
		case len(out) == 0:
			out = append(out, t.Muted.Render("no harness installed"))
		}
		if h.next.Kept == "" {
			out = append(out, t.Muted.Render("no local setup kept"))
		}
		if !h.next.LastTry.IsZero() {
			out = append(out, t.Muted.Render("last try: ")+t.Value.Render(h.next.LastTryModel)+t.Muted.Render(", "+h.next.LastTry.Format("2 Jan")))
		}
		return out
	}}
}

func (h home) thisMachine() column {
	return column{"THIS MACHINE", func(h home) []string {
		t := h.env.theme
		a := h.env.assessment
		models, disk := 0, uint64(0)
		for _, rt := range a.Scan.Runtimes {
			for _, m := range rt.Models {
				if m.Parent == "" {
					models++
				}
			}
		}
		disk = h.env.inventory.DiskBytes
		line := format.Plural(models, "model", "models")
		if disk > 0 {
			line += " · " + format.Bytes(disk)
		}
		out := []string{t.Value.Render(line)}
		running := t.Muted.Render("no runtime running")
		for _, rt := range a.Scan.Runtimes {
			if rt.Running {
				running = t.Value.Render(rt.Name + " running")
				break
			}
		}
		out = append(out, running)
		if free := a.Scan.Machine.Disk.FreeBytes; free > 0 {
			out = append(out, t.Value.Render(format.Bytes(free))+t.Muted.Render(" free for downloads"))
		}
		return out
	}}
}

func (h home) yourClaude() column {
	return column{"YOUR CLAUDE", func(h home) []string {
		t := h.env.theme
		a := h.env.assessment
		p := a.Scan.Usage
		if p.Sessions == 0 {
			return []string{t.Muted.Render("no sessions found")}
		}
		sessions := format.Plural(p.Sessions, "session", "sessions")
		// The span is the history's, not the priced window's, which can be
		// shorter when early sessions carry no token counts.
		if months := p.Last.Sub(p.First).Hours() / 24 / daysPerMonth; months >= 1 {
			sessions += " in " + format.Plural(int(math.Round(months)), "month", "months")
		}
		out := []string{t.Value.Render(sessions)}
		if usd := a.Cost.TotalUSD; usd > 0 {
			out = append(out, t.Value.Render(format.USD(usd))+t.Muted.Render(" at API prices"))
		}
		if usd := a.Cost.Last30DaysUSD; usd > 0 {
			out = append(out, t.Value.Render(format.USD(usd))+t.Muted.Render(" in the last 30 days"))
		}
		return out
	}}
}
