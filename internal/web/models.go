package web

import (
	"math"
	"net/http"
	"strconv"

	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/ui/report"
)

// modelsView is the Models page: every model lo0 knows, how much of the
// user's work it fits and how fast it runs here.
type modelsView struct {
	Basis   string
	Filters []filter
	Rows    []modelRow
	Empty   string
}

// filter is one of the Models page's views of the list.
type filter struct {
	Label, Href string
	Count       int
	Current     bool
}

// modelRow is one model in the list. A model too large for this machine
// has only its name, tier and the memory it needs.
type modelRow struct {
	Href, Name, Tier      string
	Installed, TooLarge   bool
	Sessions, SessionTone string
	SessionShare          float64
	Speed, Pace, PaceTone string
	Memory                string
	MemoryShare           float64
}

const (
	showFits      = "fits"
	showInstalled = "installed"
	showAll       = "all"
)

func (s *Server) models(r *http.Request, snap *snapshot) (any, error) {
	m := snap.assessment.Match
	show := r.URL.Query().Get("show")
	if show != showInstalled && show != showAll {
		show = showFits
	}
	installed := 0
	for _, c := range m.Candidates {
		if c.Installed != nil {
			installed++
		}
	}
	v := modelsView{Basis: basis(m)}
	for _, f := range []filter{
		{Label: "Fits", Href: "/models", Count: len(m.Candidates)},
		{Label: "Installed", Href: "/models?show=" + showInstalled, Count: installed},
		{Label: "All", Href: "/models?show=" + showAll, Count: len(m.Candidates) + len(m.TooLarge)},
	} {
		f.Current = f.Label == map[string]string{showFits: "Fits", showInstalled: "Installed", showAll: "All"}[show]
		v.Filters = append(v.Filters, f)
	}
	budget := float64(m.Budget.Bytes)
	for _, c := range m.Candidates {
		if show == showInstalled && c.Installed == nil {
			continue
		}
		v.Rows = append(v.Rows, candidateRow(c, budget))
	}
	if show == showAll {
		for _, t := range m.TooLarge {
			v.Rows = append(v.Rows, modelRow{
				Href: "/models/" + t.Model.ID, Name: t.Model.Name, Tier: string(t.Model.Tier), TooLarge: true,
				Memory: "needs " + format.Bytes(uint64(t.NeedsBytes)), MemoryShare: 1,
			})
		}
	}
	if len(v.Rows) == 0 {
		v.Empty = "None of the models that fit this machine is installed yet."
		if show == showFits {
			v.Empty = "No model lo0 knows fits this machine with a useful context window."
		}
	}
	return v, nil
}

func candidateRow(c match.Candidate, budget float64) modelRow {
	row := modelRow{
		Href: "/models/" + c.Model.ID, Name: c.Model.Name, Tier: string(c.Model.Tier), Installed: c.Installed != nil,
		Sessions: format.Percent(c.Best.Coverage), SessionShare: c.Best.Coverage, SessionTone: coverageTone(c.Best.Coverage),
		Speed:  strconv.Itoa(int(c.Best.TokensPerSecond)) + " tok/s",
		Memory: format.Bytes(uint64(c.Best.MemoryBytes)),
	}
	if budget > 0 {
		row.MemoryShare = float64(c.Best.MemoryBytes) / budget
	}
	row.Pace, row.PaceTone = pace(c.Pace)
	return row
}

// pace is a candidate's speed against what the user is used to, as a
// multiple, and its colour. The words carry the meaning without colour.
func pace(p match.Pace) (string, string) {
	tone := paceTone(p.Band)
	switch {
	case p.Band == match.BandVerySlow:
		return "very slow", tone
	case p.Band == match.BandSlower && p.Slower > 0:
		return format.Multiple(p.Slower) + " slower", tone
	case p.Band == match.BandSlower:
		return "slow", tone
	case p.Slower >= match.AboutAsFast:
		return format.Multiple(p.Slower) + " slower", tone
	case p.Slower > 0:
		return "about as fast", tone
	default:
		return "", tone
	}
}

// basis says what the list is measured against: the memory, the bandwidth
// and the hosted model the user is used to.
func basis(m match.Result) string {
	parts := []string{report.BudgetPhrase(m.Budget)}
	if m.Bandwidth.GBps > 0 {
		parts = append(parts, strconv.Itoa(int(math.Round(m.Bandwidth.GBps)))+" GB/s")
	}
	if len(m.References) > 0 {
		r := m.References[0]
		parts = append(parts, "against "+r.Name+" at "+strconv.Itoa(int(math.Round(r.TokensPerSecond)))+" tok/s")
	}
	return join(parts)
}

// join separates short facts with a middle dot.
func join(parts []string) string {
	out := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if out != "" {
			out += " · "
		}
		out += p
	}
	return out
}
