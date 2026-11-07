package match

import (
	"slices"

	"lo0.ai/lo0/internal/catalog"
)

// Offload is which layers' experts stay in system memory beside a discrete
// GPU. The zero value keeps every weight on the GPU.
type Offload struct {
	// Layers is how many layers keep their experts in system memory.
	Layers int `json:"layers"`
	// Leading is llama.cpp's --n-cpu-moe value for Layers.
	Leading int `json:"leading"`
	// Bytes is the expert weights held in system memory.
	Bytes int64 `json:"bytes"`
}

// lo0 cannot read memory speed or link width, so it assumes slow ones.
const (
	// HostGBps is dual-channel DDR4-3200.
	HostGBps  = fallbackCPUGBps
	hostWrite = bandwidthOnlyWrite * moeWrite
	// TransferGBps is PCIe 4.0 x8, over which experts are copied for prompts.
	TransferGBps = 16
	// promptBatch is llama-server's default batch, each of which copies every
	// offloaded expert to the GPU.
	promptBatch = 512
)

// offload adjusts c, a GPU-only curve, for experts kept in system memory.
func offload(c Curve, m catalog.Model, q catalog.Quant, o Offload) Curve {
	e := m.Experts
	if o.Bytes <= 0 || e.Count == 0 {
		return c
	}
	weights := float64(m.ActiveParams) * q.BitsPerWeight / 8
	host := float64(o.Bytes) * float64(e.PerToken) / float64(e.Count)
	if weights > 0 {
		c.Write *= max(weights-host, 0) / weights
	}
	c.Write += host / (hostWrite * HostGBps * 1e9)
	c.Read += float64(o.Bytes) / (TransferGBps * 1e9 * promptBatch)
	return c
}

// offloaded returns the offload of m at q and kv with the most utility, if
// it beats full, the GPU-only option.
func (f fitter) offloaded(m catalog.Model, q catalog.Quant, kv catalog.KVType, full Option) (Option, bool) {
	layers := m.Experts.LayerCount()
	if f.host <= 0 || layers == 0 || full.MaxContext >= m.ContextLength {
		return Option{}, false
	}
	var top Option
	found := false
	for n := 1; n <= layers; n++ {
		o := f.option(m, q, kv, n)
		if o.Offload.Bytes > f.host {
			break
		}
		if o.MaxContext < minUsefulContext {
			continue
		}
		if !found || o.Utility() > top.Utility() {
			top, found = o, true
		}
	}
	if !found || full.MaxContext >= minUsefulContext && top.Utility() <= full.Utility() {
		return Option{}, false
	}
	return top, true
}

// WeightBytesOnGPU is the weights not kept in system memory.
func (o Option) WeightBytesOnGPU() int64 {
	return o.Quant.Bytes - o.Offload.Bytes
}

// BestOnGPU is the best option that keeps every weight on the GPU.
func (c Candidate) BestOnGPU() (Option, bool) {
	if !c.Best.Offloads() {
		return c.Best, true
	}
	return best(slices.DeleteFunc(slices.Clone(c.Options), Option.Offloads))
}
