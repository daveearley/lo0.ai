package report

import (
	"time"

	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/ui/draw"
)

const (
	maxProjectsShown = 5
	projectBarCells  = 12
)

// projectsSection measures each project and each month against the best
// model's window: which work could move first, and whether sessions are
// getting longer or shorter.
func projectsSection(d *doc, m match.Result, short func(string) string) {
	best, ok := m.Best()
	if !ok || len(m.Projects)+len(m.Months) == 0 {
		return
	}
	d.heading("Where " + best.Model.Name + " fits")
	if len(m.Projects) > 0 {
		shown := m.Projects[:min(len(m.Projects), maxProjectsShown)]
		rows := make([][]string, 0, len(shown))
		for _, p := range shown {
			filled, track := draw.Bar(p.Fits, projectBarCells)
			rows = append(rows, []string{
				short(p.Repo),
				format.Plural(p.Sessions, "session", "sessions"),
				coverage(d, p.Fits),
				fitStyle(d, p.Fits)(filled) + d.t.Muted.Render(track),
			})
		}
		d.table("Projects", []align{left, right, right, left}, rows)
		if extra := len(m.Projects) - len(shown); extra > 0 {
			d.more(d.t.Muted.Render("and " + format.Plural(extra, "more project", "more projects")))
		}
	}
	if len(m.Months) > 1 {
		parts := make([]string, 0, len(m.Months))
		for _, mf := range m.Months {
			parts = append(parts, d.t.Muted.Render(shortMonth(mf.Month)+" ")+coverage(d, mf.Fits))
		}
		d.row("By month", d.fit(parts))
		d.more(d.t.Muted.Render("share of sessions that fit in " + format.Tokens(best.Best.MaxContext)))
	}
}

func fitStyle(d *doc, share float64) func(...string) string {
	switch match.FitOf(share) {
	case match.FitGood:
		return d.t.Good.Render
	case match.FitPartial:
		return d.t.Warn.Render
	default:
		return d.t.Bad.Render
	}
}

// shortMonth turns "2026-10" into "Oct".
func shortMonth(m string) string {
	t, err := time.Parse("2006-01", m)
	if err != nil {
		return m
	}
	return t.Format("Jan")
}
