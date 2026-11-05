package match

import (
	"cmp"
	"maps"
	"slices"

	"lo0.ai/lo0/internal/stats"
	"lo0.ai/lo0/internal/usage"
)

// ProjectFit is how much of one project's work fits the recommended model's
// context window: the projects that fit best are the ones to move first.
type ProjectFit struct {
	Repo     string  `json:"repo"`
	Sessions int     `json:"sessions"`
	Fits     float64 `json:"fits"`
}

// MonthFit is one calendar month of sessions measured against the
// recommended model's window, to show whether sessions are getting longer.
type MonthFit struct {
	// Month is "2006-01" in local time.
	Month      string  `json:"month"`
	Sessions   int     `json:"sessions"`
	MedianPeak int     `json:"medianPeak"`
	Fits       float64 `json:"fits"`
}

// projectFits groups sessions by repository, busiest first. Sessions outside
// any repository are left out.
func projectFits(records []usage.SessionRecord, window int) []ProjectFit {
	peaks := map[string][]int{}
	for _, r := range records {
		if r.Repo != "" {
			peaks[r.Repo] = append(peaks[r.Repo], r.PeakContext)
		}
	}
	out := make([]ProjectFit, 0, len(peaks))
	for repo, ps := range peaks {
		out = append(out, ProjectFit{Repo: repo, Sessions: len(ps), Fits: within(ps, window)})
	}
	slices.SortFunc(out, func(a, b ProjectFit) int {
		return cmp.Or(cmp.Compare(b.Sessions, a.Sessions), cmp.Compare(a.Repo, b.Repo))
	})
	return out
}

// monthFits groups sessions by the month they started, oldest first.
func monthFits(records []usage.SessionRecord, window int) []MonthFit {
	peaks := map[string][]int{}
	for _, r := range records {
		if !r.Start.IsZero() {
			m := r.Start.Local().Format("2006-01")
			peaks[m] = append(peaks[m], r.PeakContext)
		}
	}
	out := make([]MonthFit, 0, len(peaks))
	for _, m := range slices.Sorted(maps.Keys(peaks)) {
		ps := peaks[m]
		out = append(out, MonthFit{Month: m, Sessions: len(ps), MedianPeak: stats.Percentile(ps, 50), Fits: within(ps, window)})
	}
	return out
}

func within(peaks []int, window int) float64 {
	n := 0
	for _, p := range peaks {
		if p <= window {
			n++
		}
	}
	return stats.Share(n, len(peaks))
}
