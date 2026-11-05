package match

import (
	"cmp"

	"lo0.ai/lo0/internal/catalog"
)

// Background is a second, smaller model for a harness's background work,
// such as session titles, summaries and quick subagents, run alongside the
// main model so that work does not wait on it.
type Background struct {
	Model catalog.Model `json:"model"`
	// Option is how to run it in the memory the main model leaves. Its
	// Coverage is zero: background requests are short, so the user's
	// session sizes say nothing about them.
	Option Option `json:"option"`
}

// backgroundMaxParams caps the size of a background model. A mixture of
// experts decodes as fast as a small model but would take most of the
// memory the main model leaves, and load far more slowly for a title that
// is wanted at once.
const backgroundMaxParams = 9e9

// BackgroundFor picks the model to run alongside main's recommended option:
// a candidate of tier C or better under backgroundMaxParams that fits in the
// memory main leaves at a reliable quantization and is faster than main.
// Harnesses give every model the same window, so one whose window reaches
// main's comes first, then the fastest. It reports false when none fits.
func (r Result) BackgroundFor(main Candidate) (Background, bool) {
	f := fitter{
		budget:    int64(r.Budget.Bytes) - main.Best.MemoryBytes,
		reference: r.ReferenceContext,
		speeds:    r.speeds,
		kvTypes:   r.kvTypes,
	}
	var top Background
	found := false
	for _, c := range r.Candidates {
		if c.Model.ID == main.Model.ID || tierWeight[c.Model.Tier] == 0 || float64(c.Model.Params) >= backgroundMaxParams {
			continue
		}
		m := c.Model
		m.ContextLength = min(m.ContextLength, main.Best.MaxContext)
		o, ok := backgroundOption(f.options(m), m.ContextLength)
		if !ok || o.TokensPerSecond <= main.Best.TokensPerSecond {
			continue
		}
		bg := Background{Model: c.Model, Option: o}
		if !found || betterBackground(bg, top, main.Best.MaxContext) {
			top, found = bg, true
		}
	}
	return top, found
}

// backgroundOption is the highest-precision option with a reliable
// quantization that reaches window, or failing that the one with the
// longest window.
func backgroundOption(options []Option, window int) (Option, bool) {
	var top Option
	found := false
	for _, o := range options {
		if o.Quant.BitsPerWeight < minQualityBits {
			continue
		}
		if o.MaxContext >= window {
			return o, true
		}
		if !found || o.MaxContext > top.MaxContext {
			top, found = o, true
		}
	}
	return top, found
}

// betterBackground reports whether a beats b: a window that reaches the
// main model's first, then speed, then tier, then catalogue ID so the
// choice is stable.
func betterBackground(a, b Background, mainWindow int) bool {
	full := func(bg Background) bool { return bg.Option.MaxContext >= mainWindow }
	if full(a) != full(b) {
		return full(a)
	}
	return cmp.Or(
		cmp.Compare(a.Option.TokensPerSecond, b.Option.TokensPerSecond),
		cmp.Compare(tierWeight[a.Model.Tier], tierWeight[b.Model.Tier]),
		cmp.Compare(b.Model.ID, a.Model.ID),
	) > 0
}
