package cost

import (
	"cmp"
	"maps"
	"slices"
	"time"

	"lo0.ai/lo0/internal/claudecode/setup"
	"lo0.ai/lo0/internal/usage"
)

// daysPerMonth is the average month length, for monthly averages.
const daysPerMonth = 30.44

// Report is what the history would have cost at API prices.
type Report struct {
	// TotalUSD is the API price of every response in the history.
	TotalUSD float64 `json:"totalUsd"`
	// MonthlyUSD averages TotalUSD over the period the history covers.
	MonthlyUSD float64 `json:"monthlyUsd"`
	// Last30DaysUSD is the API price of sessions that started in the 30
	// days up to the most recent one: a fairer picture of current use than
	// an average over months of lighter use.
	Last30DaysUSD float64       `json:"last30DaysUsd"`
	Months        float64       `json:"months"`
	ByModel       []ModelCost   `json:"byModel"`
	ByMonth       []MonthCost   `json:"byMonth"`
	Plan          Plan          `json:"plan"`
	ExtraUsage    bool          `json:"extraUsage,omitzero"`
	Tokens        usage.Tokens  `json:"tokens"`
	Cache         CacheSavings  `json:"cache"`
	Local         LocalEstimate `json:"local,omitzero"`
	// Unpriced lists models with no known price; their tokens are counted
	// but not costed.
	Unpriced    []string `json:"unpriced,omitzero"`
	PricesAsOf  string   `json:"pricesAsOf"`
	PriceSource string   `json:"priceSource"`
}

// ValueMultiple is how many times over the plan's price the last 30 days'
// API value is, or zero when the plan price is not known.
func (r Report) ValueMultiple() float64 {
	if r.Plan.MonthlyUSD <= 0 {
		return 0
	}
	return r.Last30DaysUSD / r.Plan.MonthlyUSD
}

// ModelCost is the spend on one model.
type ModelCost struct {
	Model  string       `json:"model"`
	USD    float64      `json:"usd"`
	Tokens usage.Tokens `json:"tokens"`
	Priced bool         `json:"priced"`
}

// MonthCost is the spend in one calendar month, by session start.
type MonthCost struct {
	Month    string  `json:"month"`
	USD      float64 `json:"usd"`
	Sessions int     `json:"sessions"`
}

// CacheSavings compares what the prompt cache cost with sending the same
// tokens uncached.
type CacheSavings struct {
	SavedUSD float64 `json:"savedUsd"`
	// ReadShare is the share of prompt tokens served from the cache.
	ReadShare float64 `json:"readShare"`
}

// Local is the model a local setup would use, from the match.
type Local struct {
	Model           string
	Window          int
	TokensPerSecond float64
}

// LocalEstimate is what a local model could take over.
type LocalEstimate struct {
	Model string `json:"model"`
	// USD is the API price of the sessions whose peak context fits Local's
	// window.
	USD   float64 `json:"usd"`
	Share float64 `json:"share"`
	// SessionShare is the share of sessions that fit.
	SessionShare float64 `json:"sessionShare"`
	// MonthlyUSD averages USD over the history's period.
	MonthlyUSD float64 `json:"monthlyUsd"`
	// OutputHours is how long the local model would take just to generate
	// every output token in the history, at its estimated speed.
	OutputHours float64 `json:"outputHours"`
}

// Estimate prices the profile's token usage, compares it with the plan in
// acct and, if local names a model, estimates what it could take over.
func Estimate(p usage.Profile, acct setup.Account, local Local) Report {
	r := Report{Plan: PlanFor(acct), ExtraUsage: acct.ExtraUsage, PricesAsOf: PricesAsOf, PriceSource: PricesSource}

	byModel := map[string]*ModelCost{}
	unpriced := map[string]bool{}
	for _, u := range p.Usage {
		usd, priced := price(u)
		mc := byModel[u.Model]
		if mc == nil {
			mc = &ModelCost{Model: u.Model, Priced: priced}
			byModel[u.Model] = mc
		}
		mc.USD += usd
		mc.Tokens = mc.Tokens.Add(u.Tokens)
		r.Tokens = r.Tokens.Add(u.Tokens)
		r.TotalUSD += usd
		if !priced {
			unpriced[u.Model] = true
		}
		r.Cache.SavedUSD += cacheSaving(u)
	}
	for _, name := range slices.Sorted(maps.Keys(byModel)) {
		r.ByModel = append(r.ByModel, *byModel[name])
	}
	slices.SortStableFunc(r.ByModel, func(a, b ModelCost) int { return cmp.Compare(b.USD, a.USD) })
	r.Unpriced = slices.Sorted(maps.Keys(unpriced))
	if prompt := r.Tokens.Prompt(); prompt > 0 {
		r.Cache.ReadShare = float64(r.Tokens.CacheRead) / float64(prompt)
	}

	r.Months = months(p.First, p.Last)
	if r.Months > 0 {
		r.MonthlyUSD = r.TotalUSD / r.Months
	}
	r.ByMonth = byMonth(p.Records)
	r.Last30DaysUSD = lastDays(p.Records, 30)
	if local.Model != "" {
		r.Local = estimateLocal(p, local, r)
	}
	return r
}

// price returns the API price of one usage entry and whether the model has
// a known price.
func price(u usage.ModelUsage) (float64, bool) {
	rates, ok := RatesFor(u.Model, u.LongPrompt)
	if !ok {
		return 0, false
	}
	if u.Fast {
		rates = rates.Scale(fastMultiplier)
	}
	if u.USOnly {
		rates = rates.Scale(usOnlyMultiplier)
	}
	t := u.Tokens
	return (float64(t.Input)*rates.Input + float64(t.CacheWrite5m)*rates.CacheWrite5m +
		float64(t.CacheWrite1h)*rates.CacheWrite1h + float64(t.CacheRead)*rates.CacheRead +
		float64(t.Output)*rates.Output) / 1e6, true
}

// cacheSaving is the difference between sending cached tokens as fresh input
// and what the cache actually cost, including the write premium.
func cacheSaving(u usage.ModelUsage) float64 {
	rates, ok := RatesFor(u.Model, u.LongPrompt)
	if !ok {
		return 0
	}
	t := u.Tokens
	saved := float64(t.CacheRead) * (rates.Input - rates.CacheRead)
	premium := float64(t.CacheWrite5m)*(rates.CacheWrite5m-rates.Input) + float64(t.CacheWrite1h)*(rates.CacheWrite1h-rates.Input)
	return (saved - premium) / 1e6
}

// months is the span from first to last in months, at least one, so a
// history of a single day averages to its own total.
func months(first, last time.Time) float64 {
	if first.IsZero() || last.IsZero() || last.Before(first) {
		return 0
	}
	return max(last.Sub(first).Hours()/24/daysPerMonth, 1)
}

func byMonth(records []usage.SessionRecord) []MonthCost {
	m := map[string]*MonthCost{}
	for _, rec := range records {
		if rec.Start.IsZero() {
			continue
		}
		key := rec.Start.Local().Format("2006-01")
		mc := m[key]
		if mc == nil {
			mc = &MonthCost{Month: key}
			m[key] = mc
		}
		mc.Sessions++
		for _, u := range rec.Usage {
			usd, _ := price(u)
			mc.USD += usd
		}
	}
	out := make([]MonthCost, 0, len(m))
	for _, k := range slices.Sorted(maps.Keys(m)) {
		out = append(out, *m[k])
	}
	return out
}

// lastDays sums the price of sessions that started in the n days up to the
// latest session start.
func lastDays(records []usage.SessionRecord, n int) float64 {
	var latest time.Time
	for _, rec := range records {
		if rec.Start.After(latest) {
			latest = rec.Start
		}
	}
	cutoff := latest.AddDate(0, 0, -n)
	total := 0.0
	for _, rec := range records {
		if rec.Start.After(cutoff) {
			for _, u := range rec.Usage {
				usd, _ := price(u)
				total += usd
			}
		}
	}
	return total
}

func estimateLocal(p usage.Profile, local Local, r Report) LocalEstimate {
	e := LocalEstimate{Model: local.Model}
	fits := 0
	for _, rec := range p.Records {
		if rec.PeakContext > local.Window {
			continue
		}
		fits++
		for _, u := range rec.Usage {
			usd, _ := price(u)
			e.USD += usd
		}
	}
	if r.TotalUSD > 0 {
		e.Share = e.USD / r.TotalUSD
	}
	if len(p.Records) > 0 {
		e.SessionShare = float64(fits) / float64(len(p.Records))
	}
	if r.Months > 0 {
		e.MonthlyUSD = e.USD / r.Months
	}
	if local.TokensPerSecond > 0 {
		e.OutputHours = float64(r.Tokens.Output) / local.TokensPerSecond / 3600
	}
	return e
}
