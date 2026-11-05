package tui

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"lo0.ai/lo0/internal/format"
)

// command is one palette entry.
type command struct {
	kind  string
	title string
	// note is shown muted after the title and is searched too.
	note string
	run  tea.Cmd
}

// palette searches screens, models and actions.
type palette struct {
	env      env
	open     bool
	input    textinput.Model
	commands []command
	matches  []int
	cursor   int
	// titleW is the width of the title column, so notes line up.
	titleW int
}

func newPalette(e env) palette {
	in := textinput.New()
	in.Prompt = "› "
	in.Placeholder = "Go to a screen, a model or an action"
	p := palette{env: e, input: in}
	for _, id := range screenOrder {
		p.commands = append(p.commands, command{kind: "screen", title: screenNames[id], note: screenQuestions[id], run: navigate(id, nil)})
	}
	if best, ok := e.assessment.Match.Best(); ok {
		target := e.assessment.Match.Target
		if e.try {
			p.commands = append(p.commands, command{kind: "action", title: "Try " + best.Model.Name + " for one session", note: "in " + target, run: try(best.Model.ID)})
		}
		if e.change {
			id := e.assessment.Match.TargetID
			p.commands = append(p.commands, command{kind: "action", title: "Keep " + best.Model.Name, note: "for good in " + target, run: func() tea.Msg { return keepInMsg{harness: id} }})
		}
	}
	p.commands = append(p.commands, command{kind: "action", title: "Compare the three best models", run: navigate(screenModels, compareTopMsg{})})
	if e.change && len(e.inventory.Changes) > 0 {
		p.commands = append(p.commands, command{kind: "action", title: "Undo what lo0 changed", note: "see the plan first", run: func() tea.Msg { return undoMsg{} }})
	}
	if e.export {
		p.commands = append(p.commands, command{kind: "action", title: "Save the report to share", note: "plain text, project paths replaced", run: func() tea.Msg { return exportMsg{} }})
	}
	for _, c := range e.assessment.Match.Candidates {
		// The note carries what tells models apart: many fit the same share
		// of sessions, few run at the same speed in the same memory.
		note := "fits " + format.Percent(c.Best.Coverage) + " · " + strconv.Itoa(int(c.Best.TokensPerSecond)) + " tok/s · " + format.Bytes(uint64(max(c.Best.MemoryBytes, 0)))
		if c.Installed != nil {
			note += " · installed"
		}
		p.commands = append(p.commands, command{kind: "model", title: c.Model.Name, note: note, run: navigate(screenModels, selectModelMsg{id: c.Model.ID})})
	}
	for _, t := range e.assessment.Match.TooLarge {
		p.commands = append(p.commands, command{kind: "model", title: t.Model.Name, note: "too large", run: navigate(screenModels, selectModelMsg{id: t.Model.ID})})
	}
	for _, c := range p.commands {
		p.titleW = max(p.titleW, lipgloss.Width(c.title))
	}
	p.titleW = min(p.titleW, maxPaletteTitle)
	p.filter()
	return p
}

// maxPaletteTitle caps the title column, so one long title does not push
// every note off a narrow window.
const maxPaletteTitle = 36

func (p *palette) setWidth(w int) { p.input.SetWidth(max(w-4, 10)) }

func (p *palette) show() tea.Cmd {
	p.open = true
	p.input.SetValue("")
	p.cursor = 0
	p.filter()
	return p.input.Focus()
}

func (p *palette) close() {
	p.open = false
	p.input.Blur()
}

func (p *palette) filter() {
	q := strings.ToLower(strings.TrimSpace(p.input.Value()))
	p.matches = make([]int, 0, len(p.commands))
	for i, c := range p.commands {
		if q == "" || strings.Contains(strings.ToLower(c.kind+" "+c.title+" "+c.note), q) {
			p.matches = append(p.matches, i)
		}
	}
	p.cursor = min(p.cursor, max(len(p.matches)-1, 0))
}

func (p palette) update(msg tea.KeyPressMsg) (palette, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+k":
		p.close()
		return p, nil
	case "up", "ctrl+p":
		p.cursor = max(p.cursor-1, 0)
		return p, nil
	case "down", "ctrl+n":
		p.cursor = min(p.cursor+1, max(len(p.matches)-1, 0))
		return p, nil
	case "enter":
		if len(p.matches) == 0 {
			return p, nil
		}
		run := p.commands[p.matches[p.cursor]].run
		p.close()
		return p, run
	}
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	p.filter()
	return p, cmd
}

func (p palette) view(height int) string {
	t := p.env.theme
	lines := []string{p.input.View(), ""}
	room := max(height-3, 1)
	start := max(0, p.cursor-room+1)
	for i := start; i < len(p.matches) && i < start+room; i++ {
		c := p.commands[p.matches[i]]
		kind := t.Muted.Render(padRight(c.kind, 8))
		note := ""
		if c.note != "" {
			note = t.Muted.Render("   " + c.note)
		}
		title := padRight(ansi.Truncate(c.title, p.titleW, "…"), p.titleW)
		if i == p.cursor {
			lines = append(lines, t.Accent.Render("▌ ")+kind+t.Accent.Bold(true).Render(title)+note)
		} else {
			lines = append(lines, "  "+kind+t.Value.Render(title)+note)
		}
	}
	if len(p.matches) == 0 {
		lines = append(lines, t.Muted.Render("  Nothing matches."))
	} else {
		lines = append(lines, "", t.Muted.Render("  "+format.Int(len(p.matches))+" of "+format.Int(len(p.commands))))
	}
	return strings.Join(lines, "\n")
}
