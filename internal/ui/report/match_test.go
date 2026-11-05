package report_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/claudecode/setup"
	"lo0.ai/lo0/internal/cost"
	"lo0.ai/lo0/internal/harness/claude"
	"lo0.ai/lo0/internal/harness/opencode"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/scan"
	"lo0.ai/lo0/internal/ui/report"
	"lo0.ai/lo0/internal/ui/theme"
	"lo0.ai/lo0/internal/usage"
)

func harnesses() []match.Harness {
	return []match.Harness{claude.Harness{}, opencode.Harness{}}
}

func sampleCatalog() *catalog.Catalog {
	model := func(id string, tier catalog.Tier, params, active int64, ctx, layers int, ollama ...string) catalog.Model {
		return catalog.Model{
			ID: id, Name: id, Params: params, ActiveParams: active, ContextLength: ctx, Ollama: ollama,
			Attention: catalog.Attention{FullLayers: layers, FullKV: 1024},
			Quants: []catalog.Quant{
				{Name: "Q8_0", Bytes: params * 85 / 80, BitsPerWeight: 8.5},
				{Name: "Q4_K_M", Bytes: params * 485 / 800, BitsPerWeight: 4.85},
			},
			Tier: tier,
		}
	}
	return &catalog.Catalog{Generated: "2026-11-04", Models: []catalog.Model{
		model("Qwen3-Coder-30B-A3B", catalog.TierA, 30_500_000_000, 3_400_000_000, 262144, 48, "qwen3-coder:30b"),
		model("Hybrid-Coder-80B-A3B", catalog.TierA, 79_700_000_000, 3_900_000_000, 262144, 12),
		model("Dense-General-27B", catalog.TierB, 27_800_000_000, 27_800_000_000, 131072, 16),
		model("Giant-480B", catalog.TierA, 480_000_000_000, 35_000_000_000, 262144, 62),
		model("Small-2B", catalog.TierC, 2_300_000_000, 2_300_000_000, 262144, 6),
	}}
}

func sampleMatch() (scan.Result, match.Result) {
	res := sampleResult()
	peaks := make([]int, 0, 100)
	for i := range 100 {
		peaks = append(peaks, 20_000+i*9_000)
	}
	res.Usage.Context.Peaks = peaks
	res.Usage.Context.P50 = peaks[50]
	res.Usage.Context.P90 = peaks[90]
	res.Usage.MCPServers = []usage.Tally{{Name: "Claude_Browser", Count: 4741}, {Name: "claude-in-chrome", Count: 2941}, {Name: "github", Count: 65}}
	res.Setup.MCPServers = append(res.Setup.MCPServers, setup.MCPServer{Name: "github", Transport: setup.TransportHTTP})
	res.Usage.Speeds = []usage.Speed{
		{Harness: "Claude Code", Model: "claude-opus-5-5", Replies: 22_974, Recent: 19_500, TokensPerSecond: 96, Low: 85, High: 108, ReplyTokens: 565, ReplySeconds: 6.1},
		{Harness: "Codex", Model: "gpt-5.5", Replies: 1_023, TokensPerSecond: 45, Low: 40, High: 48, ReplyTokens: 405, ReplySeconds: 9.6},
	}
	repos := []string{under("Code/app"), under("Code/app"), under("Code/site"), ""}
	for i, peak := range peaks {
		res.Usage.Records = append(res.Usage.Records, usage.SessionRecord{
			Start:       res.Usage.Last.AddDate(0, 0, -i),
			PeakContext: peak,
			Repo:        repos[i%len(repos)],
			Usage:       []usage.ModelUsage{{Model: "claude-opus-5-5", Tokens: usage.Tokens{CacheRead: int64(peak) * 300, Output: 200e3}}},
		})
	}
	return res, match.Match(res, sampleCatalog(), harnesses())
}

func sampleAssessment() app.Assessment {
	res, m := sampleMatch()
	res.Setup.Account = setup.Account{Plan: setup.PlanMax, Tier: "5x", ExtraUsage: true}
	res.Usage.Usage = []usage.ModelUsage{
		{Model: "claude-opus-5-5", Tokens: usage.Tokens{Input: 1e6, CacheWrite5m: 50e6, CacheWrite1h: 60e6, CacheRead: 9e9, Output: 30e6}},
		{Model: "claude-fable-5", Tokens: usage.Tokens{CacheWrite1h: 20e6, CacheRead: 2e9, Output: 8e6}},
	}
	best, _ := m.Best()
	return app.Assessment{Scan: res, Match: m, Cost: cost.Estimate(res.Usage, res.Setup.Account,
		cost.Local{Model: best.Model.Name, Window: best.Best.MaxContext, TokensPerSecond: best.Best.TokensPerSecond})}
}

func TestRenderMatchGolden(t *testing.T) {
	a := sampleAssessment()
	for _, width := range []int{80, 60} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			got := filepath.ToSlash(report.RenderMatch(a, report.Options{Width: width, Theme: theme.Plain(), Home: home()}))
			golden := filepath.Join("testdata", "match_"+strconv.Itoa(width)+".golden")
			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("reading golden file (run go test -update): %v", err)
			}
			if got != string(want) {
				t.Errorf("match report differs from %s; run go test ./internal/ui/report -update and review the diff\n\ngot:\n%s", golden, got)
			}
		})
	}
}

func TestRenderMatchFitsWidth(t *testing.T) {
	res, m := sampleMatch()
	for _, width := range []int{60, 80, 120} {
		out := report.RenderMatch(app.Assessment{Scan: res, Match: m}, report.Options{Width: width, Theme: theme.New(false), Home: home()})
		limit := min(width, report.MaxWidth)
		for i, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
			if w := lipgloss.Width(line); w > limit {
				t.Errorf("width %d: line %d is %d columns: %q", width, i+1, w, line)
			}
		}
	}
}

func TestRenderMatchWithNoFit(t *testing.T) {
	res, _ := sampleMatch()
	res.Machine.Memory.TotalBytes = 4 << 30
	m := match.Match(res, sampleCatalog(), harnesses())
	out := report.RenderMatch(app.Assessment{Scan: res, Match: m}, report.Options{Width: 80, Theme: theme.Plain()})
	for _, want := range []string{"Not yet", "none with a useful context window", "Too large"} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
}

func TestCostSection(t *testing.T) {
	a := sampleAssessment()
	out := report.RenderSection(report.SectionCost, a, plain(80))
	for _, want := range []string{
		"What your Claude Code use is worth", "API value", "Max at $100 a month",
		"extra usage is on", "read from the cache", "Opus 5.5", "Going local", "non-stop", "5 Nov 2026",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("cost section missing %q:\n%s", want, out)
		}
	}
	if out := report.RenderSection(report.SectionCost, app.Assessment{}, plain(80)); out != "" {
		t.Errorf("cost section with no usage = %q, want nothing", out)
	}
}

func TestCarryoverWithNoSetupSaysSo(t *testing.T) {
	res, m := sampleMatch()
	m.Features = nil
	out := report.RenderMatch(app.Assessment{Scan: res, Match: m}, report.Options{Width: 80, Theme: theme.Plain()})
	if !strings.Contains(out, "lo0 found no Claude Code setup to carry over") {
		t.Errorf("an empty carry-over section is not explained:\n%s", out)
	}
}
