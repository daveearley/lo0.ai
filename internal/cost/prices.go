// Package cost works out what a user's Claude Code history would have cost
// at API prices, what their subscription costs, and how much of that a
// local model could take over.
package cost

import (
	"strings"

	"lo0.ai/lo0/internal/claudecode/setup"
)

// Rates are API prices in US dollars per million tokens.
type Rates struct {
	Input        float64 `json:"input"`
	CacheWrite5m float64 `json:"cacheWrite5m"`
	CacheWrite1h float64 `json:"cacheWrite1h"`
	CacheRead    float64 `json:"cacheRead"`
	Output       float64 `json:"output"`
}

// Scale multiplies every rate by f.
func (r Rates) Scale(f float64) Rates {
	return Rates{r.Input * f, r.CacheWrite5m * f, r.CacheWrite1h * f, r.CacheRead * f, r.Output * f}
}

// Where the prices come from and when they were taken.
const (
	PricesSource = "https://platform.claude.com/docs/en/about-claude/pricing"
	PricesAsOf   = "2026-11-05"
)

// Pricing modifiers from the pricing page.
const (
	// fastMultiplier: fast mode costs twice the standard rate in every
	// category on the models that offer it.
	fastMultiplier = 2
	// usOnlyMultiplier applies to US-only inference on Claude 4.6 and later.
	usOnlyMultiplier = 1.1
)

// prices are first-party API rates. Claude 4.6 and later bill the full 1M
// context at these standard rates; Haiku 5.5 is the exception, priced higher
// above a 100k prompt (longPrompt below).
var prices = map[string]Rates{
	"claude-fable-5-1":  {10, 12.50, 20, 0.25, 50},
	"claude-mythos-5-1": {10, 12.50, 20, 0.25, 50},
	"claude-fable-5":    {10, 12.50, 20, 1, 50},
	"claude-mythos-5":   {10, 12.50, 20, 1, 50},
	"claude-opus-5-5":   {4, 5, 8, 0.20, 20},
	"claude-opus-5":     {5, 6.25, 10, 0.50, 25},
	"claude-opus-4-8":   {5, 6.25, 10, 0.50, 25},
	"claude-opus-4-7":   {5, 6.25, 10, 0.50, 25},
	"claude-opus-4-6":   {5, 6.25, 10, 0.50, 25},
	"claude-opus-4-5":   {5, 6.25, 10, 0.50, 25},
	"claude-opus-4-1":   {15, 18.75, 30, 1.50, 75},
	"claude-opus-4":     {15, 18.75, 30, 1.50, 75},
	"claude-sonnet-5-5": {2, 2.50, 4, 0.10, 10},
	"claude-sonnet-5":   {2, 2.50, 4, 0.20, 10},
	"claude-sonnet-4-6": {3, 3.75, 6, 0.30, 15},
	"claude-sonnet-4-5": {3, 3.75, 6, 0.30, 15},
	"claude-sonnet-4":   {3, 3.75, 6, 0.30, 15},
	"claude-haiku-5-5":  {0.10, 0.125, 0.20, 0.01, 0.50},
	"claude-haiku-4-5":  {1, 1.25, 2, 0.10, 5},
	"claude-3-5-haiku":  {0.80, 1, 1.60, 0.08, 4},
}

// longPrompt are rates for prompts over 100,000 tokens, where they differ.
var longPrompt = map[string]Rates{
	"claude-haiku-5-5": {0.50, 0.625, 1, 0.05, 2.50},
}

// RatesFor returns the rates for a model as it appears in a transcript.
// Model names may carry suffixes such as a date or "[1m]"; the longest known
// prefix wins.
func RatesFor(model string, long bool) (Rates, bool) {
	id := strings.ToLower(model)
	if i := strings.IndexByte(id, '['); i >= 0 {
		id = id[:i]
	}
	best := ""
	for known := range prices {
		if (id == known || strings.HasPrefix(id, known+"-")) && len(known) > len(best) {
			best = known
		}
	}
	if best == "" {
		return Rates{}, false
	}
	if r, ok := longPrompt[best]; ok && long {
		return r, true
	}
	return prices[best], true
}

// Plan is a subscription and what it costs.
type Plan struct {
	Name       string  `json:"name"`
	MonthlyUSD float64 `json:"monthlyUsd,omitzero"`
	// Known is false when lo0 could not tell the plan or does not know its
	// price, for example team and enterprise seats.
	Known bool `json:"known"`
}

// PlanFor names the plan and its list price from claude.com/pricing. Since
// 30 October 2026 the $100 plan is called Max and the $200 one, formerly
// Max 20x, Ultra; lo0 tells them apart by the account's 20x tier.
func PlanFor(a setup.Account) Plan {
	switch a.Plan {
	case setup.PlanPro:
		return Plan{Name: "Pro", MonthlyUSD: 20, Known: true}
	case setup.PlanMax:
		if a.Tier == "20x" {
			return Plan{Name: "Ultra", MonthlyUSD: 200, Known: true}
		}
		return Plan{Name: "Max", MonthlyUSD: 100, Known: true}
	case setup.PlanAPI:
		return Plan{Name: "API key", Known: true}
	case setup.PlanTeam:
		return Plan{Name: "Team"}
	case setup.PlanEnterprise:
		return Plan{Name: "Enterprise"}
	default:
		return Plan{Name: "unknown"}
	}
}
