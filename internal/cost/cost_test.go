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
		model string
		long  bool
		input float64
		ok    bool
	}{
		{"claude-opus-5-5", false, 4, true},
		{"claude-opus-5", false, 5, true},
		{"claude-opus-5-5[1m]", false, 4, true},
		{"claude-haiku-4-5-20251001", false, 1, true},
		{"claude-haiku-5-5", false, 0.10, true},
		{"claude-haiku-5-5", true, 0.50, true},
		{"claude-fable-5-1", false, 10, true},
		{"claude-mythos-5", false, 10, true},
		{"claude-opus-4-20250514", false, 15, true},
		{"claude-opus-4-8", false, 5, true},
		{"claude-3-5-haiku-20241022", false, 0.80, true},
		{"gpt-5", false, 0, false},
	}
	for _, tt := range tests {
		r, ok := cost.RatesFor(tt.model, tt.long)
		if ok != tt.ok || r.Input != tt.input {
			t.Errorf("RatesFor(%q, %v) = %v, %v; want input %v, %v", tt.model, tt.long, r.Input, ok, tt.input, tt.ok)
		}
	}
	if r, _ := cost.RatesFor("claude-fable-5-1", false); r.CacheRead != 0.25 {
		t.Errorf("Fable 5.1 cache read = %v, want 0.025x of input", r.CacheRead)
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
		{setup.Account{Plan: setup.PlanAPI}, cost.Plan{Name: "API key", Known: true}},
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
	r := cost.Estimate(p, setup.Account{Plan: setup.PlanMax, Tier: "5x"}, cost.Local{Model: "Local-30B", Window: 256 << 10, TokensPerSecond: 25})

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
	if r.PricesAsOf == "" || r.PriceSource == "" {
		t.Error("report does not say where its prices come from")
	}
}

func TestEstimateUSOnlyAndEmpty(t *testing.T) {
	p := usage.Profile{Usage: []usage.ModelUsage{{Model: "claude-sonnet-5-5", USOnly: true, Tokens: usage.Tokens{Output: 1_000_000}}}}
	if r := cost.Estimate(p, setup.Account{}, cost.Local{}); !near(r.TotalUSD, 11) {
		t.Errorf("US-only output = %v, want $11 (1.1x of $10)", r.TotalUSD)
	}
	empty := cost.Estimate(usage.Profile{}, setup.Account{}, cost.Local{})
	if empty.TotalUSD != 0 || empty.ValueMultiple() != 0 || empty.Local.Model != "" {
		t.Errorf("empty estimate = %+v", empty)
	}
}

// A history of one session averages to its own total, as one of an hour
// does, instead of to nothing.
func TestEstimateSingleSessionMonth(t *testing.T) {
	tokens := usage.Tokens{Output: 1_000_000}
	p := usage.Profile{First: day(3), Last: day(3), Usage: opus(tokens)}
	if r := cost.Estimate(p, setup.Account{}, cost.Local{}); !near(r.MonthlyUSD, r.TotalUSD) || r.TotalUSD == 0 {
		t.Errorf("MonthlyUSD = %v, total %v; want the total", r.MonthlyUSD, r.TotalUSD)
	}
}
