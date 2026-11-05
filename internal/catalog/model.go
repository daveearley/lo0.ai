// Package catalog is lo0's knowledge base of open models: their size,
// attention layout, context window, quantizations and how well they drive an
// agentic coding harness.
//
// The data is generated from Hugging Face model configs by
// `go generate ./internal/catalog` and embedded in the binary, so matching
// works offline and is reproducible.
package catalog

import "cmp"

// Tier rates how reliably a model drives an agentic coding loop: calling
// tools with valid arguments, recovering from tool errors and staying on task
// over long sessions.
type Tier string

// Agentic tiers, best first.
const (
	TierA Tier = "A"
	TierB Tier = "B"
	TierC Tier = "C"
	TierD Tier = "D"
)

// TierDescriptions explains each tier in a sentence.
var TierDescriptions = map[Tier]string{
	TierA: "built and evaluated for agentic coding; reliable multi-step tool use",
	TierB: "general model with dependable tool calling; weaker on long agent loops",
	TierC: "tool calling works but breaks down over many steps",
	TierD: "no usable tool calling",
}

// Rank orders tiers, lower is better. Unknown tiers rank last.
func (t Tier) Rank() int {
	switch t {
	case TierA:
		return 0
	case TierB:
		return 1
	case TierC:
		return 2
	case TierD:
		return 3
	default:
		return 4
	}
}

// Model is one catalog entry.
type Model struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Family   string `json:"family"`
	Repo     string `json:"repo"`
	License  string `json:"license,omitzero"`
	Released string `json:"released,omitzero"`
	// Ollama lists the Ollama tags that serve this model, without ":latest".
	Ollama []string `json:"ollama,omitzero"`
	// GGUF is the Hugging Face repository that publishes every quantization
	// in Quants as a GGUF file.
	GGUF string `json:"gguf,omitzero"`
	// Params counts every weight, including vision towers and embeddings.
	Params int64 `json:"params"`
	// ActiveParams counts the weights read per generated token. It equals
	// Params for dense models.
	ActiveParams int64 `json:"activeParams"`
	// StatedActiveParams is the active count the model card states, when
	// it differs from ActiveParams. Show it with StatedActive.
	StatedActiveParams int64     `json:"statedActiveParams,omitzero"`
	ContextLength      int       `json:"contextLength"`
	Attention          Attention `json:"attention"`
	Quants             []Quant   `json:"quants"`
	// Vision is true when the model can read images, from the vision
	// encoder its config declares.
	Vision bool `json:"vision,omitzero"`
	Tier   Tier `json:"tier"`
	// TierNote says what the tier means.
	TierNote string `json:"tierNote,omitzero"`
	// Profile is what the model's lab says it is for, good at and weak at,
	// as reviewed from its model card.
	Profile Profile `json:"profile,omitzero"`
}

// Profile summarises a model card. Every line must be backed by the card it
// links to; lo0 adds its own, machine-specific observations separately.
type Profile struct {
	Summary    string      `json:"summary,omitzero"`
	GoodAt     []string    `json:"goodAt,omitzero"`
	WatchOut   []string    `json:"watchOut,omitzero"`
	Benchmarks []Benchmark `json:"benchmarks,omitzero"`
	// Card is the URL the profile was taken from.
	Card string `json:"card,omitzero"`
}

// StatedActive is the active parameter count to show a user: the card's
// own figure where it gives one, otherwise the derived count.
func (m Model) StatedActive() int64 {
	return cmp.Or(m.StatedActiveParams, m.ActiveParams)
}

// MoE reports whether the model activates only some of its weights per token.
func (m Model) MoE() bool {
	return m.ActiveParams > 0 && m.ActiveParams < m.Params
}

// Quant is one weight format and the bytes it takes on disk and in memory.
type Quant struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
	// BitsPerWeight is the average precision, used to rank quality.
	BitsPerWeight float64 `json:"bitsPerWeight"`
}
