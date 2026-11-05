package match

import (
	"fmt"
	"math"
	"strings"

	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/scan"
	"lo0.ai/lo0/internal/stats"
)

// Level is the overall answer.
type Level string

// Verdict levels.
const (
	LevelReady     Level = "ready"
	LevelTradeOffs Level = "trade-offs"
	LevelNotYet    Level = "not-yet"
	// LevelNoHistory means lo0 found no sessions to judge fit by. The
	// models that fit the machine are still ranked.
	LevelNoHistory Level = "no-history"
)

// Verdict is lo0's recommendation with its reasons.
type Verdict struct {
	Level Level `json:"level"`
	// Score is 0 to 100: how much of the user's work could move today.
	Score    int    `json:"score"`
	Headline string `json:"headline"`
	// Summary is one plain sentence a person can act on.
	Summary string   `json:"summary"`
	Reasons []string `json:"reasons"`
}

// Thresholds for the verdict.
const (
	readyCoverage   = 0.85
	minCoverage     = 0.5
	minSpeed        = 10
	maxLostAppShare = 0.1
)

// Fit grades a share of sessions that fit a context window.
type Fit int

// Fit grades, worst first.
const (
	// FitPoor means a local model would have to compact most sessions.
	FitPoor Fit = iota
	FitPartial
	// FitGood is the share the verdict calls ready.
	FitGood
)

// FitOf grades share with the thresholds the verdict uses, so every front
// end colours a share the way the verdict judges it.
func FitOf(share float64) Fit {
	switch {
	case share >= readyCoverage:
		return FitGood
	case share >= minCoverage:
		return FitPartial
	default:
		return FitPoor
	}
}

func decide(r Result, res scan.Result) Verdict {
	features := harness.Summarize(r.Features, "")
	appCalls, mcpCalls := appToolCalls(r.Features), totalMCPCalls(res)
	appShare := stats.Share(appCalls, mcpCalls)

	best, ok := r.Best()
	v := Verdict{Summary: summary(r, res, best, ok), Reasons: reasons(r, res, best, ok, appCalls)}
	if !ok {
		v.Level = LevelNotYet
		v.Headline = fmt.Sprintf("No catalogued coding model fits in %s with a useful context window.", format.Bytes(r.Budget.Bytes))
		return v
	}
	// With no sessions every share is zero, which would read as "none of
	// your work fits" when lo0 simply has nothing to measure.
	if res.Usage.Sessions == 0 {
		v.Level = LevelNoHistory
		v.Headline = "lo0 found no Claude Code or Codex sessions, so it cannot say how much of your work a local model could take."
		return v
	}

	speed := min(best.Best.TokensPerSecond/comfortableSpeed, 1)
	tier := tierWeight[best.Model.Tier]
	v.Score = int(math.Round(100 * (0.45*best.Best.Coverage + 0.1*best.Best.Quality + 0.15*speed + 0.15*tier +
		0.15*(features.CarriedShare()*(1-appShare)))))

	switch {
	case best.Best.Coverage < minCoverage:
		v.Level = LevelNotYet
		v.Headline = "Most of your sessions need more context than a local model can hold on this machine."
	case best.Best.TokensPerSecond < minSpeed:
		v.Level = LevelNotYet
		v.Headline = "Models that fit would run too slowly for agentic work."
	case best.Best.Coverage >= readyCoverage && best.Model.Tier == catalog.TierA &&
		best.Best.TokensPerSecond >= comfortableSpeed && appShare < maxLostAppShare:
		v.Level = LevelReady
		v.Headline = "A local model can handle almost all of your sessions."
	default:
		v.Level = LevelTradeOffs
		v.Headline = "A local model can handle many of your sessions, with trade-offs."
	}
	return v
}

func reasons(r Result, res scan.Result, best Candidate, ok bool, appCalls int) []string {
	var out []string
	ctx := res.Usage.Context
	if ok {
		var notes []string
		if best.Best.KV == catalog.KVQ8 {
			notes = append(notes, "with an 8-bit KV cache")
		}
		switch {
		case best.UsesInstalled():
			notes = append(notes, "which you already have")
		case best.Installed != nil:
			notes = append(notes, "a different build from the "+best.Installed.Option.Quant.Name+" you have")
		}
		how := best.Best.Quant.Name
		if len(notes) > 0 {
			how += ", " + strings.Join(notes, ", ")
		}
		if res.Usage.Sessions == 0 {
			out = append(out, fmt.Sprintf("Best fit for this machine is %s at %s, with a %s window, at about %d tokens a second.",
				best.Model.Name, how, format.Tokens(best.Best.MaxContext), int(best.Best.TokensPerSecond)))
		} else {
			out = append(out, fmt.Sprintf("Best fit is %s at %s: %s of your sessions fit in its %s window, at about %d tokens a second.",
				best.Model.Name, how, format.Percent(best.Best.Coverage),
				format.Tokens(best.Best.MaxContext), int(best.Best.TokensPerSecond)))
		}
		if len(r.References) > 0 {
			out = append(out, PaceSentence(best.Pace, r.References[0]))
		}
		if best.Best.Quant.BitsPerWeight < minQualityBits {
			out = append(out, fmt.Sprintf("Only a %s quantization fits, which costs noticeable quality.", best.Best.Quant.Name))
		}
	}
	if res.Usage.Sessions > 0 {
		out = append(out, fmt.Sprintf("Your median session peaks at %s tokens and one in ten passes %s. Sessions above the window would have to compact.",
			format.Tokens(ctx.P50), format.Tokens(ctx.P90)))
	}
	if chain := res.Usage.Requests.ToolCalls.P90; chain >= longChain {
		out = append(out, fmt.Sprintf("One in ten of your requests chains %s or more tool calls before it answers. Long chains are where local models most often lose track.",
			format.Int(chain)))
	}
	if img := res.Usage.Images; ok && img.Sessions > 0 && !best.Model.Vision {
		out = append(out, fmt.Sprintf("%s of your sessions had images, mostly screenshots from tools. %s cannot read them.",
			format.Int(img.Sessions), best.Model.Name))
	}
	if browser, names := harness.AppCalls(r.Features, harness.StatusPartial); browser > 0 {
		out = append(out, fmt.Sprintf("%s calls went to the Claude apps' browser tools (%s). A browser MCP server can take over, but expect more retries from a local model.",
			format.Int(browser), strings.Join(names, ", ")))
	}
	if appCalls > 0 {
		_, names := harness.AppCalls(r.Features, harness.StatusLost)
		out = append(out, fmt.Sprintf("%s calls went to tools only the Claude apps provide, such as %s. They do not work with a local model.",
			format.Int(appCalls), strings.Join(names[:min(len(names), 2)], " and ")))
	}
	if hooks := harness.Summarize(r.Features, harness.AreaHooks); hooks.Lost > 0 {
		verb := "have"
		if hooks.Lost == 1 {
			verb = "has"
		}
		// Older Claude Code versions did not record hook runs, so "none
		// ran" is only said when the history records some.
		ran := ""
		switch runs := lostHookRuns(r.Features); {
		case runs > 0:
			ran = "; they ran " + format.Plural(runs, "time", "times") + " in your history"
		case len(setupHistory(res).HookRuns) > 0:
			ran = "; none of them ran in your history"
		}
		out = append(out, fmt.Sprintf("%s of %s hook handlers %s no equivalent in %s%s.",
			format.Int(hooks.Lost), format.Int(hooks.Total()), verb, r.Target, ran))
	}
	if r.Bandwidth.Estimated {
		out = append(out, fmt.Sprintf("Speeds are rough: lo0 does not know the memory bandwidth of %s and assumed %d GB/s.", r.Bandwidth.Device, int(r.Bandwidth.GBps)))
	}
	return out
}

// lostHookRuns counts history runs of hook events that would be lost. Runs
// are recorded per event, so each event is counted once however many
// handlers it has.
func lostHookRuns(features []harness.Feature) int {
	seen := map[string]bool{}
	n := 0
	for _, f := range features {
		if f.Area == harness.AreaHooks && f.Status == harness.StatusLost && !seen[f.Name] {
			seen[f.Name] = true
			n += f.Calls
		}
	}
	return n
}

// appToolCalls counts calls to app tools that would be lost.
func appToolCalls(features []harness.Feature) int {
	n, _ := harness.AppCalls(features, harness.StatusLost)
	return n
}

func totalMCPCalls(res scan.Result) int {
	n := 0
	for _, t := range setupHistory(res).MCPServers {
		if !harness.AppPlumbing(t.Name) {
			n += t.Count
		}
	}
	return n
}
