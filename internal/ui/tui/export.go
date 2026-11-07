package tui

import (
	"errors"
	"io/fs"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/redact"
	"lo0.ai/lo0/internal/ui/report"
	"lo0.ai/lo0/internal/ui/theme"
)

// exportWidth is the width the saved report is laid out at.
const exportWidth = 80

// maxExportAttempts bounds the numbered names tried when a report of the
// same name already exists.
const maxExportAttempts = 20

// exportMsg asks the root to offer an export, from the palette.
type exportMsg struct{}

// savedMsg reports the outcome of writing the report.
type savedMsg struct {
	name string
	err  error
}

// exportName is the file the report is saved as on a given day.
func exportName(now time.Time) string {
	return "lo0-report-" + now.Format(time.DateOnly) + ".txt"
}

// exportText is the plain match report with project paths and the home
// directory replaced, since a saved report is usually passed on.
func exportText(a app.Assessment, home string) string {
	projects := app.ProjectPaths(a.Scan)
	hide := func(s string) string { return redact.Paths(s, home, projects) }
	text := report.RenderMatch(a, report.Options{Width: exportWidth, Theme: theme.Plain(), Home: home, Hide: hide})
	return hide(ansi.Strip(text))
}

// save writes the report without replacing an existing file: if the name is
// taken it tries name-2, name-3 and so on.
func save(write func(string, []byte) error, name string, data []byte) tea.Cmd {
	return func() tea.Msg {
		base, ext := strings.TrimSuffix(name, ".txt"), ".txt"
		for i := 1; i <= maxExportAttempts; i++ {
			candidate := name
			if i > 1 {
				candidate = base + "-" + strconv.Itoa(i) + ext
			}
			err := write(candidate, data)
			if errors.Is(err, fs.ErrExist) {
				continue
			}
			return savedMsg{name: candidate, err: err}
		}
		return savedMsg{name: name, err: fs.ErrExist}
	}
}

func (m Model) canExport() bool { return m.opts.Save != nil && m.opts.Now != nil }

// handleExportKey answers the confirmation: enter saves, anything that
// means no cancels.
func (m Model) handleExportKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Open):
		m.exporting = false
		name := exportName(m.opts.Now())
		return m, save(m.opts.Save, name, []byte(exportText(m.assessment, m.opts.Home)))
	case key.Matches(msg, m.keys.Back, m.keys.Quit):
		m.exporting = false
	}
	return m, nil
}

func (m Model) exportView(width int) string {
	t := m.theme
	name := exportName(m.opts.Now())
	lines := []string{
		t.Label.Render("EXPORT"),
		"",
		t.Value.Render("Save this assessment as a plain-text report:"),
		t.Accent.Render(name) + t.Muted.Render(" in the folder you started lo0 from."),
		"",
		t.Muted.Render("Your project paths and home directory are replaced, so it can be shared."),
		t.Muted.Render("An existing file is never overwritten."),
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, width, "…")
	}
	return strings.Join(lines, "\n")
}
