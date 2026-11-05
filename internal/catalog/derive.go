package catalog

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Architecture is what lo0 derives from a Hugging Face config.json.
type Architecture struct {
	ModelType     string
	Layers        int
	Hidden        int
	ContextLength int
	Attention     Attention

	Experts         int
	ExpertsPerToken int
	ExpertFFN       int
	// ExpertWidth is the dimension experts project from: the hidden size,
	// or a smaller latent on models that route through one.
	ExpertWidth int
	// ExpertMatrices is 3 for gated MLPs (gate, up, down) and 2 for plain
	// ones such as ReLU².
	ExpertMatrices int
	MoELayers      int
	// Vision is true when the config declares a vision encoder, so the
	// model can read images such as screenshots.
	Vision bool
}

// ActiveParams estimates the weights read per token given the total
// parameter count: everything except the routed experts that are not
// selected. Embedding tables are counted as active, so the estimate errs
// towards fewer tokens per second.
func (a Architecture) ActiveParams(total int64) int64 {
	if a.Experts == 0 || a.ExpertsPerToken == 0 || a.ExpertFFN == 0 {
		return total
	}
	perExpert := int64(a.ExpertMatrices) * int64(a.ExpertWidth) * int64(a.ExpertFFN)
	idle := int64(a.MoELayers) * int64(a.Experts-a.ExpertsPerToken) * perExpert
	if idle <= 0 || idle >= total {
		return total
	}
	return total - idle
}

// hfConfig holds the config.json fields lo0 reads. Multimodal models nest
// the language model's fields under text_config.
type hfConfig struct {
	ModelType      string   `json:"model_type"`
	Layers         int      `json:"num_hidden_layers"`
	Heads          int      `json:"num_attention_heads"`
	KVHeads        int      `json:"num_key_value_heads"`
	HeadDim        int      `json:"head_dim"`
	Hidden         int      `json:"hidden_size"`
	MaxPositions   int      `json:"max_position_embeddings"`
	LayerTypes     []string `json:"layer_types"`
	SlidingWindow  *int     `json:"sliding_window"`
	UseSliding     *bool    `json:"use_sliding_window"`
	SlidingPattern int      `json:"sliding_window_pattern"`
	FullInterval   int      `json:"full_attention_interval"`
	HybridPattern  string   `json:"hybrid_override_pattern"`
	BlockTypes     []string `json:"layers_block_type"`
	MLPAct         string   `json:"mlp_hidden_act"`
	MoELatent      *int     `json:"moe_latent_size"`

	GlobalHeadDim int `json:"global_head_dim"`
	GlobalKVHeads int `json:"num_global_key_value_heads"`
	// KVSharedLayers are the last layers, which reuse the KV cache of an
	// earlier layer of the same kind instead of keeping their own.
	KVSharedLayers int `json:"num_kv_shared_layers"`

	KVLoRARank   int `json:"kv_lora_rank"`
	QKRopeDim    int `json:"qk_rope_head_dim"`
	LinearVHeads int `json:"linear_num_value_heads"`
	LinearKDim   int `json:"linear_key_head_dim"`
	LinearVDim   int `json:"linear_value_head_dim"`
	MambaHeads   int `json:"mamba_num_heads"`
	MambaHeadDim int `json:"mamba_head_dim"`
	SSMState     int `json:"ssm_state_size"`

	NumExperts    int `json:"num_experts"`
	LocalExperts  int `json:"num_local_experts"`
	RoutedExperts int `json:"n_routed_experts"`
	ExpertsPerTok int `json:"num_experts_per_tok"`
	TopKExperts   int `json:"top_k_experts"`
	MoEFFN        int `json:"moe_intermediate_size"`
	FFN           int `json:"intermediate_size"`
	FirstKDense   int `json:"first_k_dense_replace"`
}

// stateBytesPerValue is the precision runtimes keep recurrent state in.
const stateBytesPerValue = 4

// DeriveArchitecture reads a Hugging Face config.json.
func DeriveArchitecture(data []byte) (Architecture, error) {
	var outer struct {
		hfConfig
		TextConfig   json.RawMessage `json:"text_config"`
		VisionConfig json.RawMessage `json:"vision_config"`
	}
	if err := json.Unmarshal(data, &outer); err != nil {
		return Architecture{}, fmt.Errorf("parsing config: %w", err)
	}
	c := outer.hfConfig
	if len(outer.TextConfig) > 0 {
		if err := json.Unmarshal(outer.TextConfig, &c); err != nil {
			return Architecture{}, fmt.Errorf("parsing text_config: %w", err)
		}
	}
	if c.Layers == 0 || c.Heads == 0 {
		return Architecture{}, errors.New("config has no num_hidden_layers or num_attention_heads")
	}
	if c.KVHeads == 0 {
		c.KVHeads = c.Heads
	}
	if c.HeadDim == 0 {
		c.HeadDim = c.Hidden / c.Heads
	}

	a := Architecture{
		ModelType:     c.ModelType,
		Layers:        c.Layers,
		Hidden:        c.Hidden,
		ContextLength: c.MaxPositions,
		Attention:     attentionOf(c),
		Vision:        len(outer.VisionConfig) > 0 && string(outer.VisionConfig) != "null",
	}
	a.Experts, a.ExpertsPerToken, a.ExpertFFN, a.MoELayers = moeOf(c)
	a.ExpertWidth = c.Hidden
	if latent := deref(c.MoELatent); latent > 0 {
		a.ExpertWidth = latent
	}
	a.ExpertMatrices = 3
	if c.MLPAct == "relu2" {
		a.ExpertMatrices = 2
	}
	return a, nil
}

func attentionOf(c hfConfig) Attention {
	perLayer := 2 * c.KVHeads * c.HeadDim
	if c.KVLoRARank > 0 {
		perLayer = c.KVLoRARank + c.QKRopeDim
	}
	full, sliding, linear, mamba, known := layerCounts(c)
	att := Attention{FullLayers: full, FullKV: perLayer}
	if sliding > 0 {
		att.SlidingLayers = sliding
		att.SlidingKV = perLayer
		att.Window = deref(c.SlidingWindow)
		if c.GlobalHeadDim > 0 {
			att.FullKV = 2 * cmp.Or(c.GlobalKVHeads, c.KVHeads) * c.GlobalHeadDim
		}
	}
	att.StateBytes = int64(linear)*int64(c.LinearVHeads)*int64(c.LinearKDim)*int64(c.LinearVDim)*stateBytesPerValue +
		int64(mamba)*int64(c.MambaHeads)*int64(c.MambaHeadDim)*int64(c.SSMState)*stateBytesPerValue
	att.Approximate = !known
	return att
}

// layerCounts splits the layers by kind. known is false when the config
// hints at an attention scheme it does not spell out, in which case every
// layer is treated as full attention.
func layerCounts(c hfConfig) (full, sliding, linear, mamba int, known bool) {
	switch {
	case len(c.LayerTypes) > 0:
		own := c.LayerTypes[:len(c.LayerTypes)-min(max(c.KVSharedLayers, 0), len(c.LayerTypes))]
		for _, t := range own {
			switch t {
			case "full_attention":
				full++
			case "sliding_attention", "chunked_attention":
				sliding++
			case "linear_attention":
				linear++
			default:
				full++
			}
		}
		return full, sliding, linear, 0, true
	case c.FullInterval > 0:
		full = c.Layers / c.FullInterval
		return full, 0, c.Layers - full, 0, true
	case c.HybridPattern != "":
		return strings.Count(c.HybridPattern, "*"), 0, 0, strings.Count(c.HybridPattern, "M"), true
	case len(c.BlockTypes) > 0:
		for _, t := range c.BlockTypes {
			switch t {
			case "attention":
				full++
			case "mamba":
				mamba++
			}
		}
		return full, 0, 0, mamba, true
	case c.SlidingPattern > 0 && deref(c.SlidingWindow) > 0:
		full = c.Layers / c.SlidingPattern
		return full, c.Layers - full, 0, 0, true
	case deref(c.SlidingWindow) > 0 && c.UseSliding != nil && *c.UseSliding:
		return 0, c.Layers, 0, 0, true
	case deref(c.SlidingWindow) > 0 && c.UseSliding == nil:
		return c.Layers, 0, 0, 0, false
	default:
		return c.Layers, 0, 0, 0, true
	}
}

func moeOf(c hfConfig) (experts, perToken, ffn, layers int) {
	experts = firstPositive(c.NumExperts, c.RoutedExperts, c.LocalExperts)
	perToken = firstPositive(c.ExpertsPerTok, c.TopKExperts)
	if experts == 0 || perToken == 0 {
		return 0, 0, 0, 0
	}
	ffn = c.MoEFFN
	if ffn == 0 && c.LocalExperts > 0 {
		ffn = c.FFN
	}
	switch {
	case c.HybridPattern != "":
		layers = strings.Count(c.HybridPattern, "E")
	case len(c.BlockTypes) > 0:
		for _, t := range c.BlockTypes {
			if t == "moe" {
				layers++
			}
		}
	default:
		layers = c.Layers - c.FirstKDense
	}
	return experts, perToken, ffn, layers
}

func firstPositive(values ...int) int {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return 0
}

func deref(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
