package cost_test

import (
	"math"
	"testing"
	"time"

	"lo0.ai/lo0/internal/claudecode/setup"
	"lo0.ai/lo0/internal/cost"
	"lo0.ai/lo0/internal/usage"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestRatesFor(t *testing.T) {
	tests := []struct {
		model             string
		long, longContext bool
		input             float64
		provider          string
	}{
		{"claude-opus-5-5", false, false, 4, "Anthropic"},
		{"claude-opus-5", false, false, 5, "Anthropic"},
		{"claude-opus-5-5[1m]", false, false, 4, "Anthropic"},
		{"claude-haiku-4-5-20251001", false, false, 1, "Anthropic"},
		{"claude-haiku-5-5", false, false, 0.10, "Anthropic"},
		{"claude-haiku-5-5", true, false, 0.50, "Anthropic"},
		{"claude-fable-5-1", false, false, 10, "Anthropic"},
		{"claude-mythos-5", false, false, 10, "Anthropic"},
		{"claude-opus-4-20250514", false, false, 15, "Anthropic"},
		{"claude-opus-4-8", false, false, 5, "Anthropic"},
		{"claude-3-5-haiku-20241022", false, false, 0.80, "Anthropic"},
		{"gpt-5.5", false, false, 5, "OpenAI"},
		{"gpt-5.5", true, true, 10, "OpenAI"},
		{"gpt-5.5-pro", false, false, 30, "OpenAI"},
		{"gpt-5.2-codex", true, false, 1.75, "OpenAI"},
		{"gpt-5.3-codex", true, true, 1.75, "OpenAI"},
		{"gpt-6-astra", false, false, 10, "OpenAI"},
		{"gpt-6-astra", true, true, 20, "OpenAI"},
		{"gpt-5-codex", false, false, 1.25, "OpenAI"},
		{"gpt-4o", false, false, 0, ""},
		{"mystery", false, false, 0, ""},
	}
	for _, tt := range tests {
		r, list, ok := cost.RatesFor(usage.ModelUsage{Model: tt.model, LongPrompt: tt.long, LongContext: tt.longContext})
		if ok != (tt.provider != "") || r.Input != tt.input || list.Provider != tt.provider {
			t.Errorf("RatesFor(%q, %v, %v) = %v from %q, %v; want input %v from %q", tt.model, tt.long, tt.longContext, r.Input, list.Provider, ok, tt.input, tt.provider)
		}
	}
	if r, _, _ := cost.RatesFor(usage.ModelUsage{Model: "claude-fable-5-1"}); r.CacheRead != 0.25 {
		t.Errorf("Fable 5.1 cache read = %v, want 0.025x of input", r.CacheRead)
	}
	// A model the page lists no cache write price for bills writes as input.
	if r, _, _ := cost.RatesFor(usage.ModelUsage{Model: "gpt-5.2-codex"}); r.CacheWrite5m != r.Input || r.CacheRead != 0.175 || r.Output != 14 {
		t.Errorf("GPT-5.2-Codex rates = %+v", r)
	}
	for _, list := range []cost.PriceList{
		{Provider: "Anthropic", Source: cost.PricesSource, AsOf: cost.PricesAsOf},
		{Provider: "OpenAI", Source: cost.OpenAIPricesSource, AsOf: cost.OpenAIPricesAsOf},
	} {
		if _, err := time.Parse(time.DateOnly, list.AsOf); err != nil || list.Source == "" {
			t.Errorf("%s prices say %q from %q", list.Provider, list.AsOf, list.Source)
		}
	}
}

func TestChatGPTPlanFor(t *testing.T) {
	tests := []struct {
		planType string
		want     cost.Plan
	}{
		{"plus", cost.Plan{Name: "ChatGPT Plus", MonthlyUSD: 20, Known: true}},
		{"Go", cost.Plan{Name: "ChatGPT Go", MonthlyUSD: 8, Known: true}},
		{"free", cost.Plan{Name: "ChatGPT Free", Known: true}},
		{"pro", cost.Plan{Name: "ChatGPT Pro", FromUSD: 100}},
		{"team", cost.Plan{Name: "ChatGPT Business"}},
		{"business", cost.Plan{Name: "ChatGPT Business"}},
		{"enterprise", cost.Plan{Name: "ChatGPT Enterprise"}},
		{"", cost.Plan{Name: "unknown"}},
		{"someday", cost.Plan{Name: "unknown"}},
	}
	for _, tt := range tests {
		if got := cost.ChatGPTPlanFor(tt.planType); got != tt.want {
			t.Errorf("ChatGPTPlanFor(%q) = %+v, want %+v", tt.planType, got, tt.want)
		}
	}
}

func TestPlanFor(t *testing.T) {
	tests := []struct {
		acct setup.Account
		want cost.Plan
	}{
		{setup.Account{Plan: setup.PlanMax, Tier: "5x"}, cost.Plan{Name: "Max", MonthlyUSD: 100, Known: true}},
		{setup.Account{Plan: setup.PlanMax, Tier: "20x"}, cost.Plan{Name: "Ultra", MonthlyUSD: 200, Known: true}},
		{setup.Account{Plan: setup.PlanPro}, cost.Plan{Name: "Pro", MonthlyUSD: 20, Known: true}},
		{setup.Account{Plan: setup.PlanAPI}, cost.Plan{Name: "API key", Known: true, PerToken: true}},
		{setup.Account{Plan: setup.PlanTeam}, cost.Plan{Name: "Team"}},
		{setup.Account{}, cost.Plan{Name: "unknown"}},
	}
	for _, tt := range tests {
		if got := cost.PlanFor(tt.acct); got != tt.want {
			t.Errorf("PlanFor(%+v) = %+v, want %+v", tt.acct, got, tt.want)
		}
	}
}

func day(d int) time.Time { return time.Date(2026, 10, d, 12, 0, 0, 0, time.UTC) }

func opus(tokens usage.Tokens) []usage.ModelUsage {
	return []usage.ModelUsage{{Model: "claude-opus-5-5", Tokens: tokens}}
}

func TestEstimate(t *testing.T) {
	short := usage.Tokens{Input: 1_000_000, Output: 1_000_000}
	long := usage.Tokens{CacheWrite1h: 1_000_000, CacheRead: 10_000_000, Output: 1_000_000}
	p := usage.Profile{
		First: day(1), Last: day(31),
		Usage: append(opus(short.Add(long)), usage.ModelUsage{Model: "mystery-model", Tokens: usage.Tokens{Output: 5}},
			usage.ModelUsage{Model: "claude-opus-5-5", Fast: true, Tokens: usage.Tokens{Output: 1_000_000}}),
		Records: []usage.SessionRecord{
			{Start: day(2), PeakContext: 20_000, Usage: opus(short)},
			{Start: day(30), PeakContext: 500_000, Usage: opus(long)},
		},
	}
	h := cost.Harness{Name: "Claude Code", Profile: p, Plan: cost.PlanFor(setup.Account{Plan: setup.PlanMax, Tier: "5x"})}
	r := cost.Estimate(h, cost.Local{Model: "Local-30B", Window: 256 << 10, TokensPerSecond: 25})

	// short: $4 input + $20 output; long: $8 1h writes + $2 reads + $20 output;
	// fast output: $40.
	if !near(r.TotalUSD, 24+30+40) {
		t.Errorf("TotalUSD = %v, want 94", r.TotalUSD)
	}
	if len(r.Unpriced) != 1 || r.Unpriced[0] != "mystery-model" {
		t.Errorf("Unpriced = %v", r.Unpriced)
	}
	// Reads would have cost $40 as input but cost $2; the 1h writes cost $4
	// more than input would have.
	if !near(r.Cache.SavedUSD, 38-4) {
		t.Errorf("cache saved %v, want 34", r.Cache.SavedUSD)
	}
	if !near(r.Last30DaysUSD, 54) || !near(r.ValueMultiple(), 0.54) {
		t.Errorf("last 30 days = %v (x%v), want 54 (x0.54)", r.Last30DaysUSD, r.ValueMultiple())
	}
	if r.Local.Model != "Local-30B" || !near(r.Local.USD, 24) || !near(r.Local.Share, 24.0/94) || r.Local.SessionShare != 0.5 {
		t.Errorf("Local = %+v, want only the short session", r.Local)
	}
	if !near(r.Local.OutputHours, float64(r.Tokens.Output)/25/3600) {
		t.Errorf("OutputHours = %v", r.Local.OutputHours)
	}
	if len(r.ByMonth) != 1 || r.ByMonth[0].Month != "2026-10" || r.ByMonth[0].Sessions != 2 || !near(r.ByMonth[0].USD, 54) {
		t.Errorf("ByMonth = %+v", r.ByMonth)
	}
	if r.ByModel[0].Model != "claude-opus-5-5" || !r.ByModel[0].Priced {
		t.Errorf("ByModel = %+v", r.ByModel)
	}
	if r.Harness != "Claude Code" || len(r.Prices) != 1 || r.Prices[0].Provider != "Anthropic" || r.Prices[0].AsOf != cost.PricesAsOf {
		t.Errorf("report for %q says its prices are %+v, want Anthropic's alone", r.Harness, r.Prices)
	}
}

func TestEstimateUSOnlyAndEmpty(t *testing.T) {
	p := usage.Profile{Usage: []usage.ModelUsage{{Model: "claude-sonnet-5-5", USOnly: true, Tokens: usage.Tokens{Output: 1_000_000}}}}
	if r := cost.Estimate(cost.Harness{Profile: p}, cost.Local{}); !near(r.TotalUSD, 11) {
		t.Errorf("US-only output = %v, want $11 (1.1x of $10)", r.TotalUSD)
	}
	empty := cost.Estimate(cost.Harness{}, cost.Local{})
	if empty.TotalUSD != 0 || empty.ValueMultiple() != 0 || empty.Local.Model != "" {
		t.Errorf("empty estimate = %+v", empty)
	}
}

// A history of one session averages to its own total, as one of an hour
// does, instead of to nothing.
func TestEstimateSingleSessionMonth(t *testing.T) {
	tokens := usage.Tokens{Output: 1_000_000}
	p := usage.Profile{First: day(3), Last: day(3), Usage: opus(tokens)}
	if r := cost.Estimate(cost.Harness{Profile: p}, cost.Local{}); !near(r.MonthlyUSD, r.TotalUSD) || r.TotalUSD == 0 {
		t.Errorf("MonthlyUSD = %v, total %v; want the total", r.MonthlyUSD, r.TotalUSD)
	}
}

func TestSummarize(t *testing.T) {
	claude := usage.Profile{
		First: day(1), Last: day(2),
		Usage:   opus(usage.Tokens{Output: 1_000_000}),
		Records: []usage.SessionRecord{{Start: day(1), Usage: opus(usage.Tokens{Output: 1_000_000})}},
	}
	// 1M fresh input at $1.75, 4M cached at $0.175, 1M output at $14.
	codexUse := []usage.ModelUsage{{Model: "gpt-5.2-codex", LongPrompt: true, Tokens: usage.Tokens{Input: 1_000_000, CacheRead: 4_000_000, Output: 1_000_000}}}
	codex := usage.Profile{
		First: day(20), Last: day(20), Plan: "plus",
		Usage:   codexUse,
		Records: []usage.SessionRecord{{Start: day(20), Usage: codexUse}},
	}
	s := cost.Summarize([]cost.Harness{
		{Name: "Claude Code", Profile: claude, Plan: cost.PlanFor(setup.Account{Plan: setup.PlanPro})},
		{Name: "Codex", Profile: codex, Plan: cost.ChatGPTPlanFor(codex.Plan)},
		{Name: "Gemini"},
	}, cost.Local{})

	if len(s.Harnesses) != 2 || s.Harnesses[0].Harness != "Claude Code" || s.Harnesses[1].Harness != "Codex" {
		t.Fatalf("harnesses = %+v, want Claude Code then Codex and nothing for one with no tokens", s.Harnesses)
	}
	c := s.Of("Codex")
	if !near(c.TotalUSD, 1.75+0.70+14) || c.Plan.Name != "ChatGPT Plus" || !near(c.ValueMultiple(), (1.75+0.70+14)/20) {
		t.Errorf("Codex = $%v on %+v (x%v)", c.TotalUSD, c.Plan, c.ValueMultiple())
	}
	if len(c.Prices) != 1 || c.Prices[0].Provider != "OpenAI" {
		t.Errorf("Codex prices = %+v, want OpenAI's", c.Prices)
	}
	if !near(c.Cache.SavedUSD, 4*(1.75-0.175)) {
		t.Errorf("Codex cache saved %v", c.Cache.SavedUSD)
	}
	if !near(s.TotalUSD, 20+c.TotalUSD) {
		t.Errorf("TotalUSD = %v, want both harnesses", s.TotalUSD)
	}
	if !near(s.Last30DaysUSD, 20+c.TotalUSD) {
		t.Errorf("Last30DaysUSD = %v, want both harnesses' sessions", s.Last30DaysUSD)
	}
	if got := s.Of("Gemini"); got.Harness != "" {
		t.Errorf("Of(Gemini) = %+v, want nothing", got)
	}
}
