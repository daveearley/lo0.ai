package match

import (
	"cmp"
	"slices"
	"sort"
	"strings"

	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/runtimes"
	"lo0.ai/lo0/internal/usage"
)

const (
	// RuntimeOverhead covers compute buffers, the CUDA or Metal context and
	// the runtime itself.
	RuntimeOverhead = 3 << 29
	// minUsefulContext is the smallest window an agentic harness can work
	// in: its system prompt and tool definitions alone take 10–20k tokens.
	minUsefulContext = 32 << 10
	// minQualityBits marks quantizations below which tool-call formatting
	// degrades noticeably.
	minQualityBits = 3.5
	// q8KVQuality is the small quality cost of an 8-bit KV cache.
	q8KVQuality = 0.99
)

// Option is one way to run a model: a quantization and a KV cache precision.
type Option struct {
	Quant catalog.Quant  `json:"quant"`
	KV    catalog.KVType `json:"kv"`
	// MaxContext is the longest context that fits in memory, capped at the
	// model's own limit.
	MaxContext int `json:"maxContext"`
	// Coverage is the share of the user's sessions whose peak context fits
	// in MaxContext, so they would run without compacting.
	Coverage float64 `json:"coverage"`
	// TokensPerSecond is the writing speed at the reference context, and
	// ReadTokensPerSecond the speed of reading new tokens there, such as a
	// tool result. Curve says where both come from.
	TokensPerSecond     float64 `json:"tokensPerSecond"`
	ReadTokensPerSecond float64 `json:"readTokensPerSecond"`
	Curve               Curve   `json:"curve"`
	// MemoryBytes is weights, KV cache at MaxContext and RuntimeOverhead.
	MemoryBytes int64 `json:"memoryBytes"`
	// KVBytes is the KV cache at MaxContext.
	KVBytes int64 `json:"kvBytes"`
	// Quality is the share of full-precision quality the quantization and KV
	// cache keep, from 0 to 1.
	Quality float64 `json:"quality"`
}

// Utility is how much of the user's work this option could do well:
// sessions covered, scaled by quality and by how comfortable the speed is.
func (o Option) Utility() float64 {
	speed := min(o.TokensPerSecond/comfortableSpeed, 1)
	return o.Coverage * o.Quality * (0.5 + 0.5*speed)
}

// qualityPoints map bits per weight to retained quality. They follow the
// published llama.cpp perplexity measurements closely enough to rank
// options: little is lost down to about 5 bits, then losses grow quickly.
var qualityPoints = []struct{ bits, quality float64 }{
	{2.5, 0.5}, {3.0, 0.7}, {3.9, 0.88}, {4.8, 0.965}, {5.6, 0.985}, {6.5, 1},
}

// quality interpolates qualityPoints.
func quality(bits float64, kv catalog.KVType) float64 {
	q := qualityPoints[len(qualityPoints)-1].quality
	if bits < qualityPoints[0].bits {
		q = qualityPoints[0].quality
	}
	for i := 1; i < len(qualityPoints); i++ {
		lo, hi := qualityPoints[i-1], qualityPoints[i]
		if bits >= lo.bits && bits < hi.bits {
			q = lo.quality + (bits-lo.bits)/(hi.bits-lo.bits)*(hi.quality-lo.quality)
			break
		}
	}
	if kv == catalog.KVQ8 {
		q *= q8KVQuality
	}
	return q
}

// Candidate is a catalogued model evaluated against the machine and history.
type Candidate struct {
	Model catalog.Model `json:"model"`
	// Best is the recommended way to run it.
	Best Option `json:"best"`
	// Options lists every way to run it with a useful context window,
	// highest precision first.
	Options []Option `json:"options"`
	// Installed is set when the user already has the model, and describes
	// the copy they have.
	Installed *Installed `json:"installed,omitzero"`
	Score     float64    `json:"score"`
	// Pace compares Best's speed with what the user is used to.
	Pace Pace `json:"pace"`
	// Insights are lo0's own observations for this machine and history.
	Insights Insights `json:"insights"`
}

// Installed is a model the user already has in a local runtime.
type Installed struct {
	Runtime runtimes.ID `json:"runtime"`
	Tag     string      `json:"tag"`
	Option  Option      `json:"option"`
	// Capabilities are what the runtime says this build can do, such as
	// thinking. Empty when the runtime does not say.
	Capabilities []runtimes.Capability `json:"capabilities,omitzero"`
}

// TooLarge is a catalogued model that does not fit with useful context.
type TooLarge struct {
	Model catalog.Model `json:"model"`
	// NeedsBytes is the memory the smallest acceptable quantization needs
	// with NeedsContext tokens of context, the least an agent can work in.
	NeedsBytes   int64 `json:"needsBytes"`
	NeedsContext int   `json:"needsContext"`
}

type fitter struct {
	budget    int64
	context   usage.Context
	reference int
	speeds    speeds
	// kvTypes are the cache precisions the runtime can be set to use.
	kvTypes []catalog.KVType
}

// option evaluates running m at quant q with KV precision kv.
func (f fitter) option(m catalog.Model, q catalog.Quant, kv catalog.KVType) Option {
	fixed := q.Bytes + RuntimeOverhead
	o := Option{Quant: q, KV: kv, Quality: quality(q.BitsPerWeight, kv)}
	if fixed+m.Attention.KVBytes(minUsefulContext, kv) > f.budget {
		o.KVBytes = m.Attention.KVBytes(minUsefulContext, kv)
		o.MemoryBytes = fixed + o.KVBytes
		return o
	}
	o.MaxContext = sort.Search(m.ContextLength, func(tokens int) bool {
		return fixed+m.Attention.KVBytes(tokens+1, kv) > f.budget
	})
	o.KVBytes = m.Attention.KVBytes(o.MaxContext, kv)
	o.MemoryBytes = fixed + o.KVBytes
	o.Coverage = f.context.Within(o.MaxContext)
	o.Curve = f.speeds.curve(m, q, kv)
	at := cacheBytes(m, kv, min(f.reference, o.MaxContext))
	o.TokensPerSecond, o.ReadTokensPerSecond = o.Curve.WriteAt(at), o.Curve.ReadAt(at)
	return o
}

// options evaluates every quantization with each KV cache precision and
// returns those with a useful context window, highest precision first.
func (f fitter) options(m catalog.Model) []Option {
	var out []Option
	for _, q := range m.Quants {
		for _, kv := range f.kvTypes {
			if o := f.option(m, q, kv); o.MaxContext >= minUsefulContext {
				out = append(out, o)
			}
		}
	}
	return out
}

// best picks the option with the highest utility. Ties go to the option
// listed first, which is the higher-precision one with an f16 KV cache.
func best(options []Option) (Option, bool) {
	if len(options) == 0 {
		return Option{}, false
	}
	top := options[0]
	for _, o := range options[1:] {
		if o.Utility() > top.Utility() {
			top = o
		}
	}
	return top, true
}

// smallestNeed is the memory the most compact acceptable quantization needs
// at the minimum useful context.
func (f fitter) smallestNeed(m catalog.Model) int64 {
	q := m.Quants[len(m.Quants)-1]
	for _, candidate := range slices.Backward(m.Quants) {
		if candidate.BitsPerWeight >= minQualityBits {
			q = candidate
			break
		}
	}
	return q.Bytes + RuntimeOverhead + m.Attention.KVBytes(minUsefulContext, catalog.KVQ8)
}

// installedOption evaluates the copy of m a runtime already has, using its
// real size on disk.
func (f fitter) installedOption(m catalog.Model, have runtimes.Model) Option {
	q := catalog.Quant{Name: cmp.Or(strings.ToUpper(have.Quantization), "unknown"), Bytes: int64(have.SizeBytes)}
	if m.Params > 0 && have.SizeBytes > 0 {
		q.BitsPerWeight = float64(have.SizeBytes) * 8 / float64(m.Params)
	}
	if q.Bytes == 0 {
		return Option{Quant: q, KV: catalog.KVF16}
	}
	best := f.option(m, q, catalog.KVF16)
	for _, kv := range f.kvTypes {
		if o := f.option(m, q, kv); o.Utility() > best.Utility() {
			best = o
		}
	}
	return best
}
