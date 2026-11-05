package match_test

import (
	"slices"
	"strings"
	"testing"

	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/claudecode/setup"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/harness/claude"
	"lo0.ai/lo0/internal/harness/opencode"
	"lo0.ai/lo0/internal/machine"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/runtimes"
	"lo0.ai/lo0/internal/scan"
	"lo0.ai/lo0/internal/usage"
)

const gib = int64(1) << 30

func harnesses() []match.Harness {
	return []match.Harness{claude.Harness{}, opencode.Harness{}}
}

func model(id string, tier catalog.Tier, q4GiB int64, kvPerToken int, ollama ...string) catalog.Model {
	return catalog.Model{
		ID: id, Name: id, Params: 30e9, ActiveParams: 3e9, ContextLength: 256 << 10, Ollama: ollama,
		Attention: catalog.Attention{FullLayers: 1, FullKV: kvPerToken},
		Quants: []catalog.Quant{
			{Name: "Q8_0", Bytes: q4GiB * 2 * gib, BitsPerWeight: 8.5},
			{Name: "Q4_K_M", Bytes: q4GiB * gib, BitsPerWeight: 4.85},
		},
		Tier: tier,
	}
}

func testCatalog() *catalog.Catalog {
	return &catalog.Catalog{Generated: "2026-11-04", Models: []catalog.Model{
		model("coder-a", catalog.TierA, 16, 1024, "coder:30b"),
		model("general-b", catalog.TierB, 16, 512),
		model("huge-a", catalog.TierA, 200, 1024),
		model("no-tools", catalog.TierD, 4, 512),
	}}
}

func appleResult(peaks ...int) scan.Result {
	slices.Sort(peaks)
	return scan.Result{
		Machine: machine.Profile{
			Memory: machine.Memory{TotalBytes: uint64(64 * gib), Unified: true},
			GPUs:   []machine.GPU{{Name: "Apple M5 Pro", Vendor: machine.VendorApple, Cores: 20, Unified: true}},
		},
		Usage: usage.Profile{
			Sessions: len(peaks),
			Context:  usage.Context{P50: peaks[len(peaks)/2], P90: peaks[len(peaks)*9/10], Peaks: peaks},
		},
	}
}

func TestMatchRanksAndClassifies(t *testing.T) {
	res := appleResult(20_000, 40_000, 60_000, 80_000, 100_000)
	res.Runtimes = []runtimes.Runtime{{
		ID: runtimes.Ollama, Installed: true, Running: true,
		Models: []runtimes.Model{
			{Name: "coder:30b", Quantization: "Q4_K_M", SizeBytes: uint64(17 * gib)},
			{Name: "hf.co/someone/mystery:Q4_K_M", SizeBytes: uint64(3 * gib)},
		},
	}}
	r := match.Match(res, testCatalog(), harnesses())

	if r.Budget.Bytes != uint64(48*gib) || r.Bandwidth.GBps != 307 || r.TargetID != harness.ClaudeCode || len(r.Targets) != 2 || r.CatalogDate != "2026-11-04" {
		t.Errorf("result header = budget %d, bandwidth %+v, target %q", r.Budget.Bytes, r.Bandwidth, r.Target)
	}
	ids := func(cs []match.Candidate) []string {
		var out []string
		for _, c := range cs {
			out = append(out, c.Model.ID)
		}
		return out
	}
	if got := ids(r.Candidates); !slices.Equal(got, []string{"coder-a", "general-b"}) {
		t.Errorf("candidates = %v, want tier A first and no tier D", got)
	}
	best, _ := r.Best()
	if best.Installed == nil || best.Installed.Tag != "coder:30b" || best.Installed.Option.Quant.Bytes != 17*gib {
		t.Errorf("installed = %+v, want the Ollama copy with its real size", best.Installed)
	}
	if best.Best.Coverage != 1 || best.Best.TokensPerSecond <= 0 {
		t.Errorf("best option = %+v", best.Best)
	}
	if len(r.TooLarge) != 1 || r.TooLarge[0].Model.ID != "huge-a" || r.TooLarge[0].NeedsBytes < 200*gib {
		t.Errorf("TooLarge = %+v", r.TooLarge)
	}
	if len(r.Uncatalogued) != 1 || r.Uncatalogued[0].Name != "hf.co/someone/mystery:Q4_K_M" {
		t.Errorf("Uncatalogued = %+v", r.Uncatalogued)
	}
}

func TestSmallModelsRankBelowLargerOnesOfTheirTier(t *testing.T) {
	small := model("tiny-c", catalog.TierC, 1, 256)
	small.Params, small.ActiveParams = 2e9, 2e9
	mid := model("mid-c", catalog.TierC, 3, 256)
	mid.Params, mid.ActiveParams = 9e9, 9e9
	r := match.Match(appleResult(20_000, 40_000), &catalog.Catalog{Models: []catalog.Model{small, mid}}, harnesses())
	if len(r.Candidates) != 2 || r.Candidates[0].Model.ID != "mid-c" {
		t.Fatalf("candidates = %+v, want the 9B model first", r.Candidates)
	}
	if a, b := r.Candidates[0].Best.Utility(), r.Candidates[1].Best.Utility(); a > b {
		t.Errorf("utility %v > %v: the test should have the small model fit at least as well", a, b)
	}
}

func TestVerdictLevels(t *testing.T) {
	short := []int{10_000, 20_000, 30_000, 40_000, 50_000, 60_000, 70_000, 80_000, 90_000, 100_000}
	long := []int{100_000, 300_000, 400_000, 500_000, 600_000, 700_000, 800_000, 900_000, 950_000, 1_000_000}
	tests := []struct {
		name     string
		res      scan.Result
		cat      *catalog.Catalog
		want     match.Level
		headline string
	}{
		{"short sessions on a capable Mac", appleResult(short...), testCatalog(), match.LevelReady, "almost all"},
		{"long sessions", appleResult(long...), testCatalog(), match.LevelNotYet, "more context"},
		{
			"only a tier B model fits",
			appleResult(short...),
			&catalog.Catalog{Models: []catalog.Model{model("general-b", catalog.TierB, 16, 512)}},
			match.LevelTradeOffs, "with trade-offs",
		},
		{
			"nothing fits",
			appleResult(short...),
			&catalog.Catalog{Models: []catalog.Model{model("huge-a", catalog.TierA, 200, 1024)}},
			match.LevelNotYet, "No catalogued coding model fits",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := match.Match(tt.res, tt.cat, harnesses()).Verdict
			if v.Level != tt.want || !strings.Contains(v.Headline, tt.headline) {
				t.Errorf("verdict = %s %q, want %s containing %q", v.Level, v.Headline, tt.want, tt.headline)
			}
			if v.Score < 0 || v.Score > 100 || len(v.Reasons) == 0 {
				t.Errorf("score %d, reasons %v", v.Score, v.Reasons)
			}
		})
	}
}

func TestVerdictCountsAppTools(t *testing.T) {
	res := appleResult(10_000, 20_000, 30_000)
	res.Usage.MCPServers = []usage.Tally{{Name: "917d60b0-8a5b-4ceb-8448-c4cd013721a8", Count: 900, Tools: []string{"search_threads"}}, {Name: "Claude_Browser", Count: 50}, {Name: "github", Count: 100}}
	res.Setup.MCPServers = []setup.MCPServer{{Name: "github", Transport: setup.TransportHTTP}}
	r := match.Match(res, testCatalog(), harnesses())
	if r.Verdict.Level == match.LevelReady {
		t.Error("verdict is ready although most MCP calls go to app-only tools")
	}
	has := func(parts ...string) bool {
		return slices.ContainsFunc(r.Verdict.Reasons, func(s string) bool {
			for _, p := range parts {
				if !strings.Contains(s, p) {
					return false
				}
			}
			return true
		})
	}
	if !has("900 calls", "Gmail connector", "do not work with a local model") {
		t.Errorf("reasons do not explain the lost app tools: %v", r.Verdict.Reasons)
	}
	if !has("50 calls", "Claude_Browser", "browser MCP server") {
		t.Errorf("reasons do not explain the replaceable browser tools: %v", r.Verdict.Reasons)
	}
}

func TestBrowserToolsAloneDoNotBlockReady(t *testing.T) {
	res := appleResult(10_000, 20_000, 30_000)
	res.Usage.MCPServers = []usage.Tally{{Name: "claude-in-chrome", Count: 900}}
	if v := match.Match(res, testCatalog(), harnesses()).Verdict; v.Level != match.LevelReady {
		t.Errorf("verdict = %s; replaceable browser tools should not count as lost", v.Level)
	}
}

func TestInsights(t *testing.T) {
	res := appleResult(20_000, 40_000, 60_000, 80_000, 400_000)
	res.Runtimes = []runtimes.Runtime{{ID: runtimes.Ollama, Models: []runtimes.Model{{Name: "coder:30b", Quantization: "Q8_0", SizeBytes: uint64(32 * gib)}}}}
	r := match.Match(res, testCatalog(), harnesses())
	byID := map[string]match.Candidate{}
	for _, c := range r.Candidates {
		byID[c.Model.ID] = c
	}
	coder := byID["coder-a"]
	has := func(lines []string, part string) bool {
		return slices.ContainsFunc(lines, func(s string) bool { return strings.Contains(s, part) })
	}
	if !has(coder.Insights.Strengths, "Already installed") {
		t.Errorf("coder strengths = %v", coder.Insights.Strengths)
	}
	if !has(coder.Insights.Concerns, "20% of your sessions would compact") {
		t.Errorf("coder concerns = %v, want the 400k session called out", coder.Insights.Concerns)
	}
	if general := byID["general-b"]; !has(general.Insights.Strengths, "Fastest model") {
		t.Errorf("general-b has half the KV cache and should be fastest: %v", general.Insights.Strengths)
	}
}

func TestSpeedAtFallsWithContext(t *testing.T) {
	r := match.Match(appleResult(20_000, 40_000), testCatalog(), harnesses())
	c, _ := r.Best()
	short, long := r.SpeedAt(c, 32<<10), r.SpeedAt(c, 256<<10)
	if !(short > long && long > 0) {
		t.Errorf("speed at 32k = %v, at 256k = %v; want it to fall with context", short, long)
	}
	if beyond := r.SpeedAt(c, 10<<20); beyond != r.SpeedAt(c, c.Best.MaxContext) {
		t.Errorf("speed beyond the window = %v, want it capped at the window", beyond)
	}
}

func TestSummary(t *testing.T) {
	tests := []struct {
		peaks []int
		want  string
	}{
		{[]int{10_000, 20_000}, "All of your sessions would fit on"},
		{[]int{10_000, 20_000, 30_000, 40_000, 50_000, 60_000, 400_000, 500_000, 600_000, 700_000}, "60% of your sessions could run on"},
		{[]int{900_000, 950_000}, "None of your sessions could run on"},
	}
	for _, tt := range tests {
		if got := match.Match(appleResult(tt.peaks...), testCatalog(), harnesses()).Verdict.Summary; !strings.HasPrefix(got, tt.want) {
			t.Errorf("Summary = %q, want prefix %q", got, tt.want)
		}
	}
	none := match.Match(appleResult(10_000), &catalog.Catalog{Models: []catalog.Model{model("huge-a", catalog.TierA, 200, 1024)}}, harnesses())
	if !strings.HasPrefix(none.Verdict.Summary, "None of the catalogued") {
		t.Errorf("Summary with no fit = %q", none.Verdict.Summary)
	}
}

func TestMapsToTheInstalledRelease(t *testing.T) {
	res := appleResult(10_000, 20_000)
	res.Setup.Hooks = []setup.Hook{{Event: "UserPromptSubmit", Type: "command"}}
	status := func() harness.Status {
		oc, _ := match.Match(res, testCatalog(), harnesses()).TargetByID(harness.OpenCode)
		return oc.Features[0].Status
	}
	if got := status(); got != harness.StatusPartial {
		t.Errorf("without an installed OpenCode, the prompt hook is %s, want partial as in OpenCode 2", got)
	}
	res.Harnesses = map[harness.ID]harness.Installation{harness.OpenCode: {Installed: true, Version: "1.19.5"}}
	if got := status(); got != harness.StatusLost {
		t.Errorf("with OpenCode 1 installed, the prompt hook is %s, want lost", got)
	}
}

func TestRecommendsTheHarnessMostOfTheSetupCarriesOverTo(t *testing.T) {
	res := appleResult(10_000, 20_000)
	res.Setup.Hooks = []setup.Hook{{Event: "Notification", Type: "command"}, {Event: "Stop", Type: "prompt"}}
	res.Setup.Settings = []setup.SettingsFile{{Keys: []string{"statusLine"}}}

	r := match.Match(res, testCatalog(), harnesses())
	if r.TargetID != harness.ClaudeCode || r.Target != "Claude Code on a local model" {
		t.Fatalf("recommended %s (%s), want Claude Code", r.TargetID, r.Target)
	}
	oc, ok := r.TargetByID(harness.OpenCode)
	if !ok || oc.Score >= r.Targets[0].Score {
		t.Errorf("OpenCode score %v should be below Claude Code's %v", oc.Score, r.Targets[0].Score)
	}
	if lost := harness.Summarize(oc.Features, "").Lost; lost != 3 {
		t.Errorf("OpenCode loses %d features, want both hooks and the status line", lost)
	}

	only := match.Match(res, testCatalog(), []match.Harness{opencode.Harness{}})
	if only.TargetID != harness.OpenCode || len(only.Features) != len(oc.Features) {
		t.Errorf("with one harness, the result maps to it: %s", only.TargetID)
	}
	if got, ok := r.TargetNamed("open"); !ok || got.ID != harness.OpenCode {
		t.Errorf("TargetNamed(open) = %v, %v", got.ID, ok)
	}
	if _, ok := r.TargetNamed(""); ok {
		t.Error("an empty name matched a harness")
	}
}

// With no sessions every share is zero, which must not read as "none of
// your work fits".
func TestVerdictWithoutHistory(t *testing.T) {
	res := appleResult(1)
	res.Usage = usage.Profile{}
	r := match.Match(res, testCatalog(), harnesses())
	v := r.Verdict
	if v.Level != match.LevelNoHistory || v.Score != 0 {
		t.Errorf("Verdict = %s scored %d, want %s scored 0", v.Level, v.Score, match.LevelNoHistory)
	}
	for _, s := range append([]string{v.Headline, v.Summary}, v.Reasons...) {
		if strings.Contains(s, "0%") || strings.Contains(s, "your sessions fit") {
			t.Errorf("verdict judges sessions it does not have: %q", s)
		}
	}
	if r.ReferenceContext != match.DefaultReferenceContext {
		t.Errorf("ReferenceContext = %d, want %d", r.ReferenceContext, match.DefaultReferenceContext)
	}
}
