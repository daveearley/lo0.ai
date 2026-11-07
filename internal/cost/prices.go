// Package cost works out what each harness's history would have cost at API
// prices, what the subscription behind it costs, and how much of that a
// local model could take over.
package cost

import (
	"strings"

	"lo0.ai/lo0/internal/usage"
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

// PriceList says whose API prices a figure uses, where they were copied from
// and on what date.
type PriceList struct {
	Provider string `json:"provider"`
	Source   string `json:"source"`
	AsOf     string `json:"asOf"`
}

// table is one provider's rates by model ID. longPrompt rates apply above
// usage.LongPromptTokens and longContext rates above usage.LongContextTokens.
type table struct {
	list                    PriceList
	rates                   map[string]Rates
	longPrompt, longContext map[string]Rates
}

var tables = []table{
	{list: PriceList{"Anthropic", PricesSource, PricesAsOf}, rates: prices, longPrompt: longPrompt},
	{list: PriceList{"OpenAI", OpenAIPricesSource, OpenAIPricesAsOf}, rates: openAIPrices, longContext: openAILongContext},
}

// Where Anthropic's prices come from and when they were taken.
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

// RatesFor returns the rates for a usage entry's model as it appears in a
// log, and whose price list they come from. Model names may carry suffixes
// such as a date or "[1m]"; the longest known prefix wins.
func RatesFor(u usage.ModelUsage) (Rates, PriceList, bool) {
	id := strings.ToLower(u.Model)
	if i := strings.IndexByte(id, '['); i >= 0 {
		id = id[:i]
	}
	var found *table
	best := ""
	for i := range tables {
		for known := range tables[i].rates {
			if (id == known || strings.HasPrefix(id, known+"-")) && len(known) > len(best) {
				found, best = &tables[i], known
			}
		}
	}
	if found == nil {
		return Rates{}, PriceList{}, false
	}
	if r, ok := found.longContext[best]; ok && u.LongContext {
		return r, found.list, true
	}
	if r, ok := found.longPrompt[best]; ok && u.LongPrompt {
		return r, found.list, true
	}
	return found.rates[best], found.list, true
}
