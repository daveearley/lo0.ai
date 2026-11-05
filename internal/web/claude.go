package web

import (
	"net/http"
	"path/filepath"
	"time"

	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/ui/report"
	"lo0.ai/lo0/internal/usage"
)

// Lists on the Your Claude page show at most this many entries.
const (
	topRepos = 6
	topTools = 6
)

// claudeView is the Your Claude page: how much of the user's work can run
// locally, and how they work.
type claudeView struct {
	Level, LevelTone, Score string
	// Tone colours what fits in the best model's window.
	Tone              string
	Headline, Summary string
	Reasons           []string
	Curve             curve
	HasCurve          bool
	Context           string
	Months            []monthCol
	Repos, Tools, MCP []barRow
	Models            []barRow
	Habits            []fact
	Cost              []fact
	Empty             string
}

// monthCol is one month of sessions: how many, and the share that fit.
type monthCol struct {
	Label, Sessions, Fits string
	Height, FitShare      float64
}

// barRow is one labelled bar in a list.
type barRow struct {
	Name, Title, Value, Note string
	Share                    float64
}

func (s *Server) claude(_ *http.Request, snap *snapshot) (any, error) {
	a := snap.assessment
	p, m := a.Scan.Usage, a.Match
	v := claudeView{
		Level: report.LevelLabel(m.Verdict.Level), LevelTone: levelTone(m.Verdict.Level),
		Score: report.ScoreLabel(m.Verdict), Headline: m.Verdict.Headline, Summary: m.Verdict.Summary, Reasons: m.Verdict.Reasons,
	}
	if p.Sessions == 0 {
		v.Empty = "lo0 found no Claude Code or Codex sessions on this machine."
		return v, nil
	}
	v.Tone = "bad"
	if best, ok := m.Best(); ok {
		v.Curve, v.HasCurve = contextCurve(p.Context.Peaks, best.Best.MaxContext)
		v.Tone = coverageTone(best.Best.Coverage)
	}
	v.Context = join([]string{
		"median " + format.Tokens(p.Context.P50),
		"1 in 10 pass " + format.Tokens(p.Context.P90),
		"longest " + format.Tokens(p.Context.Max),
	})
	v.Months = months(m.Months)
	v.Repos = repos(m.Projects, s.opts.Home)
	v.Tools = tallies(p.Tools, topTools)
	v.MCP = tallies(p.MCPServers, topTools)
	v.Models = tallies(p.Models, topTools)
	for i := range v.Models {
		v.Models[i].Name = format.Model(v.Models[i].Name)
	}
	v.Habits = habits(p)
	if c := a.Cost; c.TotalUSD > 0 {
		v.Cost = []fact{
			{"At API prices", []string{format.USD(c.TotalUSD), "over " + format.DateRange(p.First, p.Last)}},
			{"Last 30 days", []string{format.USD(c.Last30DaysUSD), "about " + format.USD(c.MonthlyUSD) + " a month on average"}},
		}
	}
	return v, nil
}

func levelTone(l match.Level) string {
	switch l {
	case match.LevelReady:
		return "good"
	case match.LevelNotYet:
		return "bad"
	default:
		return "warn"
	}
}

func months(ms []match.MonthFit) []monthCol {
	most := 0
	for _, mo := range ms {
		most = max(most, mo.Sessions)
	}
	out := make([]monthCol, 0, len(ms))
	for _, mo := range ms {
		label := mo.Month
		if t, err := time.Parse("2006-01", mo.Month); err == nil {
			label = t.Format("Jan")
		}
		out = append(out, monthCol{
			Label: label, Sessions: format.Int(mo.Sessions), Fits: format.Percent(mo.Fits) + " fit",
			Height: float64(mo.Sessions) / float64(max(most, 1)), FitShare: mo.Fits,
		})
	}
	return out
}

func repos(ps []match.ProjectFit, home string) []barRow {
	most := 0
	for _, p := range ps {
		most = max(most, p.Sessions)
	}
	out := make([]barRow, 0, min(len(ps), topRepos))
	for _, p := range ps[:min(len(ps), topRepos)] {
		out = append(out, barRow{
			Name: filepath.Base(p.Repo), Title: short(p.Repo, home), Value: format.Int(p.Sessions),
			Note: format.Percent(p.Fits) + " fit", Share: float64(p.Sessions) / float64(max(most, 1)),
		})
	}
	return out
}

func tallies(ts []usage.Tally, n int) []barRow {
	if len(ts) == 0 {
		return nil
	}
	most := ts[0].Count
	out := make([]barRow, 0, min(len(ts), n))
	for _, t := range ts[:min(len(ts), n)] {
		out = append(out, barRow{Name: t.Name, Title: t.Name, Value: format.Words(int64(t.Count)), Share: float64(t.Count) / float64(max(most, 1))})
	}
	return out
}

func habits(p usage.Profile) []fact {
	out := []fact{
		{"Subagents", []string{format.Percent(p.Subagents.Share) + " of sessions"}},
		{"Compaction", []string{format.Percent(p.Compaction.Share) + " of sessions"}},
	}
	if p.Reasoning.HighEffortShare > 0 {
		out = append(out, fact{"High effort", []string{format.Percent(p.Reasoning.HighEffortShare) + " of turns"}})
	}
	if len(p.Languages) > 0 {
		total := 0
		for _, l := range p.Languages {
			total += l.Count
		}
		out = append(out, fact{"Mostly", []string{p.Languages[0].Name + " · " + format.Percent(float64(p.Languages[0].Count)/float64(max(total, 1)))}})
	}
	return out
}
