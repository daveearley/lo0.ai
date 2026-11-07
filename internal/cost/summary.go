package cost

import "lo0.ai/lo0/internal/usage"

// Summary prices each harness's history on its own and all of them together.
type Summary struct {
	// Harnesses has a report for each harness that used any tokens, in the
	// order Summarize was given them.
	Harnesses []Report `json:"harnesses,omitzero"`
	// TotalUSD covers every harness, and Last30DaysUSD every session that
	// started in the 30 days up to the latest one in any harness.
	TotalUSD      float64 `json:"totalUsd"`
	Last30DaysUSD float64 `json:"last30DaysUsd"`
}

// Summarize estimates each harness's report and the totals across them.
func Summarize(harnesses []Harness, local Local) Summary {
	var (
		s       Summary
		records []usage.SessionRecord
	)
	for _, h := range harnesses {
		r := Estimate(h, local)
		if r.Tokens.Prompt()+r.Tokens.Output == 0 {
			continue
		}
		s.Harnesses = append(s.Harnesses, r)
		s.TotalUSD += r.TotalUSD
		records = append(records, h.Profile.Records...)
	}
	s.Last30DaysUSD = lastDays(records, 30)
	return s
}

// Of returns the named harness's report, or a zero one if it used no tokens.
func (s Summary) Of(harness string) Report {
	for _, r := range s.Harnesses {
		if r.Harness == harness {
			return r
		}
	}
	return Report{}
}
