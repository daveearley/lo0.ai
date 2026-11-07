package catalog

import (
	"cmp"
	"strings"
)

// Experts describes a mixture-of-experts model's routed experts, which a
// runtime can keep in system memory.
type Experts struct {
	// Params counts every routed expert's weights.
	Params int64 `json:"params"`
	// Count is the routed experts per layer; PerToken, how many a token uses.
	Count    int `json:"count"`
	PerToken int `json:"perToken"`
	// Layers has one character per layer: E with routed experts, - without.
	Layers string `json:"layers"`
}

const expertLayer = 'E'

// LayerCount is how many layers hold routed experts.
func (e Experts) LayerCount() int {
	return strings.Count(e.Layers, string(expertLayer))
}

// Leading is the llama.cpp --n-cpu-moe value that puts n layers' experts in
// system memory. The flag counts layers without experts too.
func (e Experts) Leading(n int) int {
	if n <= 0 {
		return 0
	}
	seen := 0
	for i := range len(e.Layers) {
		if e.Layers[i] == expertLayer {
			seen++
			if seen == n {
				return i + 1
			}
		}
	}
	return len(e.Layers)
}

// ExpertBytes is the size of n layers' routed experts at quantization q.
func (m Model) ExpertBytes(q Quant, n int) int64 {
	layers := m.Experts.LayerCount()
	if layers == 0 || n <= 0 {
		return 0
	}
	bits := cmp.Or(q.ExpertBitsPerWeight, q.BitsPerWeight)
	all := min(int64(float64(m.Experts.Params)*bits/8), q.Bytes)
	return all * int64(min(n, layers)) / int64(layers)
}

func (a Architecture) expertsOf() Experts {
	if a.Experts == 0 || a.ExpertsPerToken == 0 || a.ExpertFFN == 0 || a.ExpertLayers == "" {
		return Experts{}
	}
	perExpert := int64(a.ExpertMatrices) * int64(a.ExpertWidth) * int64(a.ExpertFFN)
	return Experts{
		Params:   int64(a.MoELayers) * int64(a.Experts) * perExpert,
		Count:    a.Experts,
		PerToken: a.ExpertsPerToken,
		Layers:   a.ExpertLayers,
	}
}

func expertLayersOf(c hfConfig) string {
	b := make([]byte, c.Layers)
	for i := range b {
		b[i] = '-'
	}
	switch {
	case c.HybridPattern != "":
		for i, r := range c.HybridPattern {
			if i < len(b) && r == expertLayer {
				b[i] = expertLayer
			}
		}
	case len(c.BlockTypes) > 0:
		for i, t := range c.BlockTypes {
			if i < len(b) && t == "moe" {
				b[i] = expertLayer
			}
		}
	default:
		for i := max(c.FirstKDense, 0); i < len(b); i++ {
			b[i] = expertLayer
		}
	}
	return string(b)
}
