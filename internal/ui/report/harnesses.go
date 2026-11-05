package report

import (
	"cmp"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/ui/draw"
)

// harnessBarCells is the width of the carry-over bar, shown when the table
// has room for it.
const harnessBarCells = 14

// RenderHarnesses draws each harness lo0 can set up: whether it is
// installed, how much of the setup carries over, and under it each command
// it is started with and what that command uses now. selected marks one
// harness, or none when negative.
func RenderHarnesses(views []app.HarnessView, selected int, opts Options) string {
	t := opts.Theme
	width := opts.Width
	if width <= 0 {
		width = DefaultWidth
	}
	nameW := 12
	for _, v := range views {
		nameW = max(nameW, len(v.Name))
	}
	const installedW = 14
	bar := width >= 2+nameW+2+installedW+2+harnessBarCells+1+4+13
	var b strings.Builder
	for i, v := range views {
		marker, nameStyle := "  ", t.Value
		if i == selected {
			marker, nameStyle = t.Accent.Render("▌ "), t.Accent.Bold(true)
		}
		var installed string
		switch {
		case !v.Checked:
			installed = pad("", installedW, false)
		case v.Installation.Installed:
			installed = t.Value.Render(pad(cmp.Or(v.Installation.Version, "installed"), installedW, false))
		default:
			installed = t.Warn.Render(pad("not installed", installedW, false))
		}
		line := marker + nameStyle.Render(pad(ansi.Truncate(v.Name, nameW, "…"), nameW, false)) + "  " + installed + "  "
		if bar {
			filled, track := draw.Bar(v.Fit.Score, harnessBarCells)
			line += t.Accent.Render(filled) + t.Muted.Render(track) + " "
		}
		line += t.Value.Render(pad(format.Percent(v.Fit.Score), 4, true)) + t.Muted.Render(" carries over")
		if v.Recommended && lipgloss.Width(line)+lipgloss.Width(separator+"recommended") <= width {
			line += t.Muted.Render(separator + "recommended")
		}
		b.WriteString(line + "\n")
		for _, u := range v.Installation.Uses {
			model := t.Muted.Render(u.Model)
			if u.Local {
				model = t.Value.Render(u.Model)
			}
			b.WriteString("      " + t.Label.Render(pad(u.Command, 14, false)) + model + "\n")
		}
	}
	return b.String()
}
