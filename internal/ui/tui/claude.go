package tui

import (
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/ui/draw"
	"lo0.ai/lo0/internal/ui/report"
	"lo0.ai/lo0/internal/ui/theme"
	"lo0.ai/lo0/internal/usage"
)

const (
	// histogramRows is the height of the context histogram.
	histogramRows = 6
	// maxChartWidth keeps charts readable on very wide terminals.
	maxChartWidth = 96
	monthBar      = 16
	projectBar    = 16
	maxProjects   = 5
	maxToolBars   = 6
)

// newClaude is the Your Claude screen: the user's history drawn first, then
// the report's own cost, history and setup sections, so every fact reads as
// it does in the printed report.
func newClaude(e env) screen {
	return newPage(e, func(w int) string {
		o := e.reportOptions(w)
		a := e.assessment
		return readiness(e, min(w-8, 60)) + "\n" +
			report.RenderSection(report.SectionVerdict, a, o) + "\n" +
			claudeCharts(e, min(w, maxChartWidth)) +
			report.RenderSection(report.SectionCost, a, o) + "\n" +
			report.RenderSection(report.SectionHistory, a, o) + "\n" +
			report.RenderSection(report.SectionSetup, a, o) +
			optional(report.RenderSection(report.SectionWarnings, a, o))
	})
}

// readiness draws the verdict's score as a bar with its level, the answer
// to the screen's question in one line.
func readiness(e env, width int) string {
	t := e.theme
	v := e.assessment.Match.Verdict
	if v.Level == match.LevelNoHistory {
		return levelStyle(t, v.Level).Bold(true).Render(report.LevelLabel(v.Level)) + "\n"
	}
	filled, track := draw.Bar(float64(v.Score)/100, max(width, 10))
	return levelStyle(t, v.Level).Render(filled) + t.Muted.Render(track) + "  " +
		t.Heading.Render(strconv.Itoa(v.Score)) + t.Muted.Render(" / 100") + "\n" +
		levelStyle(t, v.Level).Bold(true).Render(report.LevelLabel(v.Level)) + "\n"
}

// claudeCharts draws what has a shape: session sizes against the local
// windows, the months, the projects and the tools.
func claudeCharts(e env, width int) string {
	t := e.theme
	p := e.assessment.Scan.Usage
	m := e.assessment.Match
	if p.Sessions == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(t.Muted.Render(format.Plural(p.Sessions, "session", "sessions")+" · "+format.DateRange(p.First, p.Last)) + "\n\n")

	window := 0
	if best, ok := m.Best(); ok {
		window = best.Best.MaxContext
	}
	b.WriteString(t.Label.Render("CONTEXT PER SESSION") + "\n")
	b.WriteString(contextHistogram(t, p.Context, window, width) + "\n")
	summary := "median " + format.Tokens(p.Context.P50) + " · 1 in 10 over " + format.Tokens(p.Context.P90)
	if best, ok := m.Best(); ok {
		summary += " · " + format.Percent(best.Best.Coverage) + " fit in " + best.Model.Name + "'s " + format.Tokens(window)
	}
	b.WriteString(t.Value.Render(ansi.Truncate(summary, width, "…")) + "\n\n")

	if months := monthRows(e, width); months != "" {
		b.WriteString(months + "\n")
	}
	if projects := projectRows(e, width); projects != "" {
		b.WriteString(projects + "\n")
	}
	if tools := toolBars(t, p, width); tools != "" {
		b.WriteString(tools + "\n")
	}
	return b.String()
}

// contextHistogram draws session peaks as columns, green up to the best
// model's window and amber past it, with each common local window marked
// underneath.
func contextHistogram(t theme.Theme, ctx usage.Context, window, width int) string {
	cells := max(width-2, 10)
	limit := sessionLimit(ctx, max(window, 1))
	rows := draw.Columns(draw.Histogram(ctx.Peaks, cells, limit), histogramRows)
	split := cells
	if window > 0 {
		split = draw.Position(window, limit, cells)
	}
	var b strings.Builder
	for _, r := range rows {
		runes := []rune(r)
		b.WriteString(t.Good.Render(string(runes[:split])) + t.Warn.Render(string(runes[split:])) + "\n")
	}
	b.WriteString(t.Muted.Render(strings.Repeat("─", cells)) + "\n")

	marks := []rune(strings.Repeat(" ", cells))
	labels := []rune(strings.Repeat(" ", cells))
	place := func(at int, label string) {
		if at+len(label) > cells {
			at = cells - len(label)
		}
		// Keep a space either side, so neighbouring labels never run
		// together.
		for i := at - 1; i <= at+len(label); i++ {
			if i >= 0 && i < cells && labels[i] != ' ' {
				return
			}
		}
		for i, r := range label {
			labels[max(at+i, 0)] = r
		}
	}
	place(0, "0")
	for _, w := range usage.ContextWindows {
		if w > limit {
			break
		}
		at := draw.Position(w, limit, cells)
		marks[at] = '┊'
		place(at, format.Tokens(w))
	}
	place(cells-len(format.Tokens(limit)), format.Tokens(limit))
	b.WriteString(t.Muted.Render(string(marks)) + "\n")
	b.WriteString(t.Muted.Render(string(labels)))
	return b.String()
}

// monthRows shows each month's sessions and how many fit the best model. The
// cost section below draws what each month was worth.
func monthRows(e env, width int) string {
	t := e.theme
	months := e.assessment.Match.Months
	if len(months) < 2 {
		return ""
	}
	top := 0
	for _, mf := range months {
		top = max(top, mf.Sessions)
	}
	var b strings.Builder
	b.WriteString(padRight(t.Label.Render("BY MONTH"), 10) + padRight(t.Muted.Render("sessions"), 5+2+monthBar) + "  " +
		padLeft(t.Muted.Render("fit"), 5) + "\n")
	for _, mf := range months {
		filled, track := draw.Bar(float64(mf.Sessions)/float64(top), monthBar)
		line := padRight(t.Value.Render(monthName(mf.Month)), 10) +
			padLeft(t.Value.Render(format.Int(mf.Sessions)), 5) + "  " +
			t.Accent.Render(filled) + t.Muted.Render(track) + "  " +
			padLeft(coverageStyle(t, mf.Fits).Render(format.Percent(mf.Fits)), 5)
		b.WriteString(ansi.Truncate(line, width, "…") + "\n")
	}
	return b.String()
}

func monthName(m string) string {
	if t, err := time.Parse("2006-01", m); err == nil {
		return t.Format("Jan 2006")
	}
	return m
}

// projectRows shows the busiest projects and how much of each fits.
func projectRows(e env, width int) string {
	t := e.theme
	projects := e.assessment.Match.Projects
	if len(projects) == 0 {
		return ""
	}
	shown := projects[:min(len(projects), maxProjects)]
	top := shown[0].Sessions
	nameWidth := min(max(width-projectBar-20, 12), 28)
	var b strings.Builder
	b.WriteString(t.Label.Render("WHERE") + "\n")
	for _, p := range shown {
		filled, track := draw.Bar(float64(p.Sessions)/float64(max(top, 1)), projectBar)
		name := ansi.Truncate(shortPath(e.home, p.Repo), nameWidth, "…")
		line := padRight(t.Value.Render(name), nameWidth) + "  " +
			t.Accent.Render(filled) + t.Muted.Render(track) +
			padLeft(t.Value.Render(format.Int(p.Sessions)), 6) + "  " +
			padLeft(coverageStyle(t, p.Fits).Render(format.Percent(p.Fits)), 5) + t.Muted.Render(" fit")
		b.WriteString(ansi.Truncate(line, width, "…") + "\n")
	}
	if extra := len(projects) - len(shown); extra > 0 {
		b.WriteString(t.Muted.Render("and "+format.Plural(extra, "more project", "more projects")) + "\n")
	}
	return b.String()
}

// toolBars draws the most used built-in tools and MCP servers.
func toolBars(t theme.Theme, p usage.Profile, width int) string {
	var b strings.Builder
	for _, group := range []struct {
		label   string
		tallies []usage.Tally
		name    func(usage.Tally) string
	}{
		{"TOOLS", p.Tools, func(t usage.Tally) string { return t.Name }},
		{"MCP", visibleServers(p.MCPServers), harness.ServerName},
	} {
		if len(group.tallies) == 0 {
			continue
		}
		items := make([]string, 0, maxToolBars)
		top := group.tallies[0].Count
		for _, tl := range group.tallies[:min(len(group.tallies), maxToolBars)] {
			filled, _ := draw.Bar(float64(tl.Count)/float64(max(top, 1)), 8)
			items = append(items, t.Value.Render(group.name(tl))+" "+t.Accent.Render(filled)+" "+t.Muted.Render(compact(tl.Count)))
		}
		label := padRight(t.Label.Render(group.label), 7)
		b.WriteString(label + atoms(items, "   ", width-7, strings.Repeat(" ", 7)) + "\n")
	}
	return b.String()
}

// visibleServers leaves out the desktop app's own interface servers, which
// are plumbing rather than tools the user chose.
func visibleServers(ts []usage.Tally) []usage.Tally {
	out := make([]usage.Tally, 0, len(ts))
	for _, s := range ts {
		if !harness.AppPlumbing(s.Name) {
			out = append(out, s)
		}
	}
	return out
}

// compact writes counts as 35k or 3.8k so bars line up in a row.
func compact(n int) string {
	switch {
	case n >= 10_000:
		return format.Int((n+500)/1000) + "k"
	case n >= 1000:
		return strings.TrimSuffix(format.Int(n/1000)+"."+format.Int(n%1000/100), ".0") + "k"
	default:
		return format.Int(n)
	}
}

// shortPath shows paths under home with "~".
func shortPath(home, p string) string {
	if home != "" {
		if rel, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.Join("~", rel)
		}
	}
	return p
}
