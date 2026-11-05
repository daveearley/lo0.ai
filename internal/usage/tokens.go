package usage

import (
	"cmp"
	"maps"
	"slices"
)

// Tokens counts tokens by the categories the API prices separately.
type Tokens struct {
	Input        int64 `json:"input"`
	CacheWrite5m int64 `json:"cacheWrite5m,omitzero"`
	CacheWrite1h int64 `json:"cacheWrite1h,omitzero"`
	CacheRead    int64 `json:"cacheRead,omitzero"`
	Output       int64 `json:"output"`
}

// Add returns the sum of t and o.
func (t Tokens) Add(o Tokens) Tokens {
	return Tokens{
		Input:        t.Input + o.Input,
		CacheWrite5m: t.CacheWrite5m + o.CacheWrite5m,
		CacheWrite1h: t.CacheWrite1h + o.CacheWrite1h,
		CacheRead:    t.CacheRead + o.CacheRead,
		Output:       t.Output + o.Output,
	}
}

// Prompt is every token the model read: fresh input, cache writes and cache
// reads.
func (t Tokens) Prompt() int64 {
	return t.Input + t.CacheWrite5m + t.CacheWrite1h + t.CacheRead
}

// ModelUsage is the tokens one model used under one pricing variant.
type ModelUsage struct {
	Model string `json:"model"`
	// Fast is true for fast mode.
	Fast bool `json:"fast,omitzero"`
	// USOnly is true for US-only inference.
	USOnly bool `json:"usOnly,omitzero"`
	// LongPrompt is true for responses whose prompt was over
	// LongPromptTokens, which some models price higher.
	LongPrompt bool   `json:"longPrompt,omitzero"`
	Tokens     Tokens `json:"tokens"`
}

// LongPromptTokens is the prompt size above which responses are tallied
// separately, so that models with long-prompt rates can be priced exactly.
const LongPromptTokens = 100_000

// Variant is a model under one pricing variant: the key token usage is
// totalled by, since each variant is priced on its own.
type Variant struct {
	Model                    string
	Fast, USOnly, LongPrompt bool
}

// Variant returns the variant u was totalled under.
func (u ModelUsage) Variant() Variant {
	return Variant{Model: u.Model, Fast: u.Fast, USOnly: u.USOnly, LongPrompt: u.LongPrompt}
}

// Totals accumulates tokens by variant.
type Totals map[Variant]Tokens

// Add counts tokens under v.
func (t Totals) Add(v Variant, tokens Tokens) {
	t[v] = t[v].Add(tokens)
}

// Usages flattens the totals into a list in a stable order.
func (t Totals) Usages() []ModelUsage {
	out := make([]ModelUsage, 0, len(t))
	for _, v := range slices.SortedFunc(maps.Keys(t), func(a, b Variant) int {
		return cmp.Or(cmp.Compare(a.Model, b.Model), compareBool(a.Fast, b.Fast), compareBool(a.USOnly, b.USOnly), compareBool(a.LongPrompt, b.LongPrompt))
	}) {
		out = append(out, ModelUsage{Model: v.Model, Fast: v.Fast, USOnly: v.USOnly, LongPrompt: v.LongPrompt, Tokens: t[v]})
	}
	return out
}

func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case !a:
		return -1
	default:
		return 1
	}
}

// MergeUsages adds the usages in b to those in a.
func MergeUsages(a, b []ModelUsage) []ModelUsage {
	t := Totals{}
	for _, list := range [][]ModelUsage{a, b} {
		for _, u := range list {
			t.Add(u.Variant(), u.Tokens)
		}
	}
	return t.Usages()
}
