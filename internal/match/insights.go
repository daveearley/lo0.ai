package match

import (
	"fmt"
	"math"

	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/scan"
	"lo0.ai/lo0/internal/usage"
)

// Insights are lo0's own observations about a candidate on this machine and
// for this user's history, as opposed to the lab's claims in its profile.
type Insights struct {
	Strengths []string `json:"strengths,omitzero"`
	Concerns  []string `json:"concerns,omitzero"`
}

// Thresholds for insights.
const (
	// holdsSpeed is the share of its short-context speed a model must keep
	// at its longest context to be called steady.
	holdsSpeed = 0.6
	// tightMemory is the share of the model memory budget above which
	// little is left for anything else.
	tightMemory = 0.9
	// shortContext is the context speed curves start from.
	shortContext = 32 << 10
)

// SpeedAt estimates the decode speed of the candidate's recommended option at
// a context of tokens, in tokens per second.
func (r Result) SpeedAt(c Candidate, tokens int) float64 {
	return c.Best.Curve.WriteAt(cacheBytes(c.Model, c.Best.KV, min(tokens, max(c.Best.MaxContext, 1))))
}

// ReadSpeedAt is c's speed reading new tokens into a context of tokens.
func (r Result) ReadSpeedAt(c Candidate, tokens int) float64 {
	return c.Best.Curve.ReadAt(cacheBytes(c.Model, c.Best.KV, min(tokens, max(c.Best.MaxContext, 1))))
}

// insights fills in each candidate's insights. It needs every candidate at
// once because some insights compare them, and the history for those that
// depend on how the user works.
func insights(r *Result, history usage.Profile) {
	var top leaders
	for _, c := range r.Candidates {
		top.coverage = max(top.coverage, c.Best.Coverage)
		top.speed = max(top.speed, c.Best.TokensPerSecond)
	}
	for _, c := range r.Candidates {
		if c.Best.Coverage == top.coverage {
			top.coverageTies++
		}
		if c.Best.TokensPerSecond == top.speed {
			top.speedTies++
		}
	}
	for i := range r.Candidates {
		c := &r.Candidates[i]
		c.Insights = insightsFor(*r, *c, top)
		needs := needsFor(*c, history)
		c.Insights.Strengths = append(c.Insights.Strengths, needs.Strengths...)
		c.Insights.Concerns = append(c.Insights.Concerns, needs.Concerns...)
	}
}

// leaders are the best values among candidates and how many share them.
type leaders struct {
	coverage, speed         float64
	coverageTies, speedTies int
}

func insightsFor(r Result, c Candidate, top leaders) Insights {
	var in Insights
	o := c.Best
	short, long := r.SpeedAt(c, shortContext), r.SpeedAt(c, o.MaxContext)

	if c.UsesInstalled() {
		in.Strengths = append(in.Strengths, "Already installed: nothing to download")
	}
	if len(r.Candidates) > 1 && o.Coverage == top.coverage && o.Coverage > 0 {
		if top.coverageTies == 1 {
			in.Strengths = append(in.Strengths, "Fits more of your sessions than any other model here")
		} else {
			in.Strengths = append(in.Strengths, "Ties for the most sessions that fit")
		}
	}
	if len(r.Candidates) > 1 && o.TokensPerSecond == top.speed && top.speedTies == 1 {
		in.Strengths = append(in.Strengths, "Fastest model that fits this machine")
	}
	if o.Coverage >= readyCoverage {
		in.Strengths = append(in.Strengths, "Fits almost all of your sessions without compacting")
	}
	if short > 0 && long >= holdsSpeed*short && long >= 2*minSpeed && o.MaxContext > shortContext*2 {
		in.Strengths = append(in.Strengths, fmt.Sprintf("Keeps its speed at long context: about %d tok/s at %s",
			int(math.Round(long)), format.Tokens(o.MaxContext)))
	}

	if exceed := 1 - o.Coverage; exceed > 0.005 {
		in.Concerns = append(in.Concerns, fmt.Sprintf("Holds %s: %s of your sessions would compact",
			format.Tokens(o.MaxContext), format.Percent(exceed)))
	}
	if long < minSpeed {
		in.Concerns = append(in.Concerns, fmt.Sprintf("Slows to about %d tok/s near %s", int(math.Round(long)), format.Tokens(o.MaxContext)))
	}
	if o.Quant.BitsPerWeight > 0 && o.Quant.BitsPerWeight < minQualityBits+0.5 {
		in.Concerns = append(in.Concerns, fmt.Sprintf("Only %s fits here, which costs some quality", o.Quant.Name))
	}
	if r.Budget.Bytes > 0 && float64(o.MemoryBytes) >= tightMemory*float64(r.Budget.Bytes) {
		in.Concerns = append(in.Concerns, fmt.Sprintf("Uses %s of %s: little room for anything else",
			format.Bytes(uint64(o.MemoryBytes)), format.Bytes(r.Budget.Bytes)))
	}
	if c.Model.Tier == catalog.TierC {
		in.Concerns = append(in.Concerns, "Tool calling tends to drift over long sessions")
	}
	return in
}

// summary is the one sentence that answers "can I switch?".
func summary(r Result, res scan.Result, best Candidate, ok bool) string {
	if !ok {
		return fmt.Sprintf("None of the catalogued coding models fit in %s with enough context to work in.", format.Bytes(r.Budget.Bytes))
	}
	if res.Usage.Sessions == 0 {
		return best.Model.Name + " fits this machine best. Run lo0 match again after some sessions in Claude Code or Codex to see how much of your work it could take."
	}
	// The share is a percentage, as it is everywhere else lo0 states it, so
	// the summary never reads as a different number from the tables.
	switch format.Percent(best.Best.Coverage) {
	case "100%":
		return fmt.Sprintf("All of your sessions would fit on %s on this machine today.", best.Model.Name)
	case "0%":
		return fmt.Sprintf("None of your sessions could run on %s on this machine today. They all need more context than it can hold, so they would have to compact.", best.Model.Name)
	}
	return fmt.Sprintf("%s of your sessions could run on %s on this machine today. The rest need more context than it can hold, so they would have to compact.",
		format.Percent(best.Best.Coverage), best.Model.Name)
}
