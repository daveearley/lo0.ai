package match

import (
	"cmp"
	"fmt"
	"math"
	"strings"

	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/machine"
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

	onCPU := r.Budget.Source == machine.MemoryCPU
	switch {
	case best.Best.Coverage < minCoverage:
		v.Level = LevelNotYet
		v.Headline = "Most of your sessions need more context than a local model can hold on this machine."
	case best.Best.TokensPerSecond < minSpeed && onCPU:
		v.Level = LevelNotYet
		v.Headline = "This machine has no GPU for models, and on the CPU the models that fit would run too slowly for agentic work."
	case best.Best.TokensPerSecond < minSpeed:
		v.Level = LevelNotYet
		v.Headline = "Models that fit would run too slowly for agentic work."
	// A CPU's speed is a rough estimate and its reading is slow, so a
	// machine without a GPU is never called ready.
	case onCPU:
		v.Level = LevelTradeOffs
		v.Headline = "This machine has no GPU for models. A local model can run on the CPU, but expect long waits."
	// Below minQualityBits tool calls start to break.
	case best.Best.Coverage >= readyCoverage && best.Model.Tier == catalog.TierA &&
		best.Best.TokensPerSecond >= comfortableSpeed && best.Best.Quant.BitsPerWeight >= minQualityBits &&
		appShare < maxLostAppShare:
		v.Level = LevelReady
		v.Headline = "A local model can handle almost all of your sessions."
	default:
		v.Level = LevelTradeOffs
		v.Headline = "A local model can handle many of your sessions, with trade-offs."
	}
	// Work that would run too slowly to use cannot move today, whatever
	// else fits, so the score shrinks with the shortfall.
	if v.Level == LevelNotYet && best.Best.TokensPerSecond < minSpeed {
		v.Score = int(math.Round(float64(v.Score) * best.Best.TokensPerSecond / minSpeed))
	}
	return v
}

func reasons(r Result, res scan.Result, best Candidate, ok bool, appCalls int) []string {
	var out []string
	onCPU := r.Budget.Source == machine.MemoryCPU
	if onCPU {
		out = append(out, noGPU(res.Machine, r.Budget))
	}
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
		if onCPU {
			out = append(out, cpuReading(best, res.Usage.Context.P50))
		}
		if off := best.Best.Offload; off.Layers > 0 {
			out = append(out, fmt.Sprintf("It fits beside the GPU's %s by keeping the experts of %d of its %d layers, %s, in system memory, which lo0 sets up with llama.cpp. Reading a prompt then copies them to the GPU, so reading is slower than writing suggests.",
				format.Bytes(r.Budget.Bytes), off.Layers, best.Model.Experts.LayerCount(), format.Bytes(uint64(off.Bytes))))
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
	switch {
	case onCPU:
		out = append(out, fmt.Sprintf("Speeds are rough: lo0 assumed %d GB/s, typical of a laptop's system memory, and CPUs vary widely. Once a model is downloaded, lo0 benchmark measures it on this machine.", int(r.Bandwidth.GBps)))
	case r.Bandwidth.Estimated:
		out = append(out, fmt.Sprintf("Speeds are rough: lo0 does not know the memory bandwidth of %s and assumed %d GB/s.", r.Bandwidth.Device, int(r.Bandwidth.GBps)))
	}
	return out
}

// noGPU says plainly that the machine has no GPU a model can run on, and
// where models would run instead.
func noGPU(p machine.Profile, budget machine.ModelMemory) string {
	where := fmt.Sprintf("so a local model would run on the CPU, from the %s of system memory lo0 leaves for models.", format.Bytes(budget.Bytes))
	if len(p.GPUs) == 0 {
		return "lo0 found no GPU on this machine, " + where
	}
	names := make([]string, 0, len(p.GPUs))
	for _, g := range p.GPUs {
		names = append(names, g.Name)
	}
	verb := "has"
	if len(names) > 1 {
		verb = "have"
	}
	return fmt.Sprintf("This machine has no GPU that can hold a model: %s %s little or no memory of its own, %s", strings.Join(names, " and "), verb, where)
}

// cpuReadContext is the context the CPU reading warning is stated at when
// there is no history to take a median from.
const cpuReadContext = 32 << 10

// cpuReading warns about the slowest part of running on a CPU: reading a
// long context, which a GPU does many times faster. tokens is the median
// session's peak, or zero without history.
func cpuReading(best Candidate, tokens int) string {
	tokens = min(cmp.Or(tokens, cpuReadContext), max(best.Best.MaxContext, 1))
	rate := best.Best.Curve.ReadCold(cacheBytes(best.Model, best.Best.KV, tokens))
	if rate <= 0 {
		return "A CPU reads long contexts far more slowly than a GPU, so expect long waits whenever a session takes in a large file or its context has to be read again."
	}
	return fmt.Sprintf("A CPU reads long contexts far more slowly than a GPU: at about %s tokens a second, reading %s tokens from scratch takes about %s. Expect that wait whenever a long session's context has to be read again, such as after the model is reloaded or the session is resumed.",
		format.Int(int(math.Round(rate))), format.Tokens(tokens), format.Seconds(float64(tokens)/rate))
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
