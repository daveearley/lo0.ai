package catalog_test

import (
	"strings"
	"testing"

	"lo0.ai/lo0/internal/catalog"
)

func TestDeriveExperts(t *testing.T) {
	tests := []struct {
		config        string
		total         int64
		count, perTok int
		layers        string
		paramsNear    float64
	}{
		{config: "Qwen_Qwen3.6-35B-A3B", total: 35_951_822_704, count: 256, perTok: 8, layers: strings.Repeat("E", 40), paramsNear: 32.2e9},
		// The first layer is dense, so it holds no routed experts.
		{config: "zai-org_GLM-4.7-Flash", total: 31_221_488_576, count: 64, perTok: 4, layers: "-" + strings.Repeat("E", 46), paramsNear: 27.8e9},
		{config: "openai_gpt-oss-20b", total: 20_914_757_184, count: 32, perTok: 4, layers: strings.Repeat("E", 24), paramsNear: 19.1e9},
		// Mamba and attention layers sit between the expert layers.
		{
			config: "nvidia_NVIDIA-Nemotron-3-Nano-30B-A3B-BF16", total: 31_577_937_344, count: 128, perTok: 6,
			layers: "-E-E--E-E-E--E-E-E--E-E-E--E-E-E--E-E-E-E--E-E-E-E-E", paramsNear: 30.7e9,
		},
		{
			config: "nvidia_NVIDIA-Nemotron-3.5-Lightning-30B-A3B-BF16", total: 31_577_937_344, count: 128, perTok: 6,
			layers: "-E-E--E-E-E--E-E-E--E-E-E--E-E-E--E-E-E-E--E-E-E-E-E", paramsNear: 30.7e9,
		},
	}
	for _, tt := range tests {
		t.Run(tt.config, func(t *testing.T) {
			arch, err := catalog.DeriveArchitecture(config(t, tt.config))
			if err != nil {
				t.Fatal(err)
			}
			m := catalog.Build(catalog.Source{ID: "x", Name: "x", Tier: catalog.TierA}, catalog.RepoInfo{Params: tt.total}, arch, "")
			e := m.Experts
			if e.Count != tt.count || e.PerToken != tt.perTok || e.Layers != tt.layers {
				t.Errorf("Experts = %d of %d per token in %q\n   want %d of %d in %q", e.PerToken, e.Count, e.Layers, tt.perTok, tt.count, tt.layers)
			}
			if p := float64(e.Params); p < tt.paramsNear*0.95 || p > tt.paramsNear*1.05 {
				t.Errorf("Experts.Params = %.1fB, want about %.1fB", p/1e9, tt.paramsNear/1e9)
			}
		})
	}
}

func TestDenseModelsHaveNoExperts(t *testing.T) {
	for _, name := range []string{"mistralai_Devstral-Small-2-24B-Instruct-2512", "google_gemma-4-E4B-it"} {
		arch, err := catalog.DeriveArchitecture(config(t, name))
		if err != nil {
			t.Fatal(err)
		}
		m := catalog.Build(catalog.Source{ID: "x", Name: "x", Tier: catalog.TierA}, catalog.RepoInfo{Params: 8e9}, arch, "")
		if m.Experts != (catalog.Experts{}) {
			t.Errorf("%s: Experts = %+v, want none", name, m.Experts)
		}
	}
}

func TestExpertsLeading(t *testing.T) {
	e := catalog.Experts{Layers: "-E-EE"}
	for n, want := range map[int]int{0: 0, 1: 2, 2: 4, 3: 5, 9: 5} {
		if got := e.Leading(n); got != want {
			t.Errorf("Leading(%d) = %d, want %d", n, got, want)
		}
	}
	if got := e.LayerCount(); got != 3 {
		t.Errorf("LayerCount = %d, want 3", got)
	}
}

func TestExpertBytes(t *testing.T) {
	m := catalog.Model{Params: 100e9, Experts: catalog.Experts{Params: 90e9, Count: 64, PerToken: 4, Layers: "EEEE"}}
	q4 := catalog.Quant{Name: "Q4", Bytes: 50e9, BitsPerWeight: 4}
	if got := m.ExpertBytes(q4, 4); got != 45e9 {
		t.Errorf("ExpertBytes(all) = %d, want 45e9", got)
	}
	if got := m.ExpertBytes(q4, 1); got != 45e9/4 {
		t.Errorf("ExpertBytes(1 of 4) = %d, want a quarter", got)
	}
	mx := catalog.Quant{Name: "MXFP4", Bytes: 60e9, BitsPerWeight: 4.8, ExpertBitsPerWeight: 4.25}
	if got := m.ExpertBytes(mx, 4); got != 47_812_500_000 {
		t.Errorf("ExpertBytes(MXFP4) = %d, want 47.8e9", got)
	}
	if got := (catalog.Model{Params: 8e9}).ExpertBytes(q4, 4); got != 0 {
		t.Errorf("dense ExpertBytes = %d, want 0", got)
	}
}
