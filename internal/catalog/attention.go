package catalog

// Attention describes how a model's KV cache grows with context. Modern
// models mix layer kinds: full attention caches every token, sliding-window
// attention caches only the most recent ones, and linear attention or state
// space layers keep a fixed-size state no matter how long the context gets.
type Attention struct {
	// FullLayers cache every token.
	FullLayers int `json:"fullLayers"`
	// FullKV is the number of cached values per token in each full layer: K
	// and V across all KV heads, or the compressed latent for MLA.
	FullKV int `json:"fullKV"`
	// SlidingLayers cache at most Window tokens.
	SlidingLayers int `json:"slidingLayers,omitzero"`
	SlidingKV     int `json:"slidingKV,omitzero"`
	Window        int `json:"window,omitzero"`
	// StateBytes is the fixed recurrent state of linear attention or Mamba
	// layers, for the whole model.
	StateBytes int64 `json:"stateBytes,omitzero"`
	// Approximate is set when the config describes an attention scheme lo0
	// cannot model exactly. The estimate then assumes full attention, which
	// overstates memory rather than understating it.
	Approximate bool `json:"approximate,omitzero"`
}

// KVType is the precision the runtime stores the KV cache in.
type KVType string

// KV cache precisions that llama.cpp and Ollama support.
const (
	KVF16 KVType = "f16"
	KVQ8  KVType = "q8_0"
)

// BytesPerValue is the storage cost of one cached value. q8_0 stores blocks
// of 32 values with one f16 scale.
func (k KVType) BytesPerValue() float64 {
	switch k {
	case KVQ8:
		return 34.0 / 32.0
	default:
		return 2
	}
}

// KVBytes returns the KV cache size for a context of tokens.
func (a Attention) KVBytes(tokens int, kv KVType) int64 {
	if tokens < 0 {
		tokens = 0
	}
	values := int64(a.FullLayers)*int64(a.FullKV)*int64(tokens) +
		int64(a.SlidingLayers)*int64(a.SlidingKV)*int64(min(tokens, a.Window))
	return int64(float64(values)*kv.BytesPerValue()) + a.StateBytes
}
