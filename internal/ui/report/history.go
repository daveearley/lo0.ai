package report

import (
	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/usage"
)

const (
	maxReposShown = 5
)

func historySection(d *doc, p usage.Profile, projectsDir string, short func(string) string) {
	d.heading("Session history")
	if p.Sessions == 0 {
		d.row("Sessions", d.t.Muted.Render("none found in "+short(projectsDir)))
		return
	}
	d.row("Sessions", d.join(format.Int(p.Sessions), format.Plural(p.Turns, "turn", "turns"), format.DateRange(p.First, p.Last)))
	harnessRows(d, p)
	d.row("Peak context", d.join(
		"p50 "+format.Tokens(p.Context.P50),
		"p90 "+d.t.Accent.Render(format.Tokens(p.Context.P90)),
		"max "+format.Tokens(p.Context.Max),
	))
	contextFit(d, p.Context.Fit)

	d.row("Compaction", d.join(
		format.Percent(p.Compaction.Share)+" of sessions",
		d.t.Muted.Render(format.Plural(p.Compaction.Total, "compaction", "compactions")+", "+format.Int(p.Compaction.Auto)+" automatic"),
	))
	d.row("Subagents", d.join(
		format.Percent(p.Subagents.Share)+" of sessions",
		d.t.Muted.Render(format.Plural(p.Subagents.Calls, "call", "calls")),
	))
	workRows(d, p)
	d.row("Tools", d.fit(counts(d, p.Tools)))
	if len(p.MCPServers) > 0 {
		d.row("MCP calls", d.fit(counts(d, p.MCPServers)))
	}
	if len(p.Languages) > 0 {
		d.row("Languages", d.fit(shares(d, p.Languages)))
	}
	if len(p.Models) > 0 {
		d.row("Models", d.fit(shares(d, p.Models)))
	}
	repos(d, p.Repos, short)
}

// contextFit shows, for each common context window, the share of sessions
// whose peak context stayed within it.
func contextFit(d *doc, fit []usage.Fit) {
	if len(fit) == 0 {
		return
	}
	header := make([]string, 0, len(fit))
	values := make([]string, 0, len(fit))
	for _, f := range fit {
		header = append(header, d.t.Muted.Render(format.Tokens(f.Tokens)))
		v := fitStyle(d, f.Share)(format.Percent(f.Share))
		values = append(values, v)
	}
	aligns := make([]align, len(fit))
	for i := range aligns {
		aligns[i] = right
	}
	d.table("Fits within", aligns, [][]string{header, values})
}

func counts(d *doc, tallies []usage.Tally) []string {
	items := make([]string, 0, len(tallies))
	for _, t := range tallies {
		items = append(items, t.Name+" "+d.t.Muted.Render(format.Int(t.Count)))
	}
	return items
}

func shares(d *doc, tallies []usage.Tally) []string {
	total := 0
	for _, t := range tallies {
		total += t.Count
	}
	items := make([]string, 0, len(tallies))
	for _, t := range tallies {
		share := 0.0
		if total > 0 {
			share = float64(t.Count) / float64(total)
		}
		items = append(items, t.Name+" "+d.t.Muted.Render(format.Percent(share)))
	}
	return items
}

func repos(d *doc, rs []usage.Repo, short func(string) string) {
	if len(rs) == 0 {
		return
	}
	shown := rs[:min(len(rs), maxReposShown)]
	rows := make([][]string, 0, len(shown))
	for _, r := range shown {
		rows = append(rows, []string{
			short(r.Path),
			format.Plural(r.Sessions, "session", "sessions"),
			d.t.Muted.Render("p90 ") + format.Tokens(r.ContextP90),
		})
	}
	d.table("Repos", []align{left, right, right}, rows)
	if extra := len(rs) - len(shown); extra > 0 {
		d.more(d.t.Muted.Render("and " + format.Plural(extra, "more repo", "more repos")))
	}
}
