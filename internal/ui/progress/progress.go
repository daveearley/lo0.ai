// Package progress is the live view shown on stderr while a scan runs. It
// clears itself when the scan finishes, so only the report remains.
package progress

import (
	"context"
	"maps"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/scan"
	"lo0.ai/lo0/internal/ui/report"
	"lo0.ai/lo0/internal/ui/theme"
)

// EventMsg delivers a scan event to the model.
type EventMsg scan.Event

// DoneMsg tells the model the scan has finished.
type DoneMsg struct{}

const labelWidth = 20

// Model is the Bubble Tea model for scan progress.
type Model struct {
	theme       theme.Theme
	spinner     spinner.Model
	stages      map[scan.Stage]scan.Event
	cancel      context.CancelFunc
	finished    bool
	interrupted bool
}

// New returns a progress model. cancel is called if the user interrupts.
func New(t theme.Theme, cancel context.CancelFunc) Model {
	return Model{
		theme:   t,
		spinner: spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(t.Accent)),
		stages:  map[scan.Stage]scan.Event{},
		cancel:  cancel,
	}
}

// WithTheme returns the model restyled with t, keeping its progress.
func (m Model) WithTheme(t theme.Theme) Model {
	m.theme = t
	m.spinner.Style = t.Accent
	return m
}

// Interrupted reports whether the user stopped the scan.
func (m Model) Interrupted() bool { return m.interrupted }

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return m.spinner.Tick
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case EventMsg:
		e := scan.Event(msg)
		prev := m.stages[e.Stage]
		if e.Total == 0 && e.Status == prev.Status {
			e.Done, e.Total = prev.Done, prev.Total
		}
		// Model is a value, so it copies the map before writing rather than
		// change one that an earlier copy still holds.
		m.stages = maps.Clone(m.stages)
		m.stages[e.Stage] = e
		return m, nil
	case DoneMsg:
		m.finished = true
		return m, tea.Quit
	case tea.InterruptMsg:
		return m.interrupt()
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m.interrupt()
		}
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) interrupt() (tea.Model, tea.Cmd) {
	m.interrupted = true
	if m.cancel != nil {
		m.cancel()
	}
	return m, tea.Quit
}

// View implements tea.Model.
func (m Model) View() tea.View {
	if m.finished || m.interrupted {
		return tea.NewView("")
	}
	var b strings.Builder
	for _, stage := range scan.Stages {
		b.WriteString(m.line(stage, m.stages[stage]))
		b.WriteByte('\n')
	}
	return tea.NewView(b.String())
}

func (m Model) line(stage scan.Stage, e scan.Event) string {
	label := report.StageLabel(stage)
	pad := strings.Repeat(" ", max(labelWidth-len(label), 1))
	switch e.Status {
	case scan.StatusDone:
		if e.Warnings > 0 {
			return "  " + m.theme.Warn.Render("!") + " " + m.theme.Value.Render(label) + pad +
				m.theme.Warn.Render(format.Plural(e.Warnings, "warning", "warnings"))
		}
		return "  " + m.theme.Good.Render("✓") + " " + m.theme.Value.Render(label) + pad + m.theme.Muted.Render(report.StageFound(stage, e.Found))
	case scan.StatusRunning:
		detail := ""
		if e.Total > 0 {
			detail = m.theme.Muted.Render(format.Int(e.Done) + " of " + format.Plural(e.Total, "session", "sessions"))
		}
		return "  " + m.spinner.View() + " " + m.theme.Value.Render(label) + pad + detail
	default:
		return "  " + m.theme.Muted.Render("·") + " " + m.theme.Muted.Render(label)
	}
}
