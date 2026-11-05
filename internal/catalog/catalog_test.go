package catalog_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"lo0.ai/lo0/internal/catalog"
)

func config(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "configs", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestDeriveArchitecture(t *testing.T) {
	tests := []struct {
		config     string
		total      int64
		want       catalog.Attention
		context    int
		activeNear float64
	}{
		{
			config:  "Qwen_Qwen3.6-35B-A3B",
			total:   35_951_822_704,
			want:    catalog.Attention{FullLayers: 10, FullKV: 1024, StateBytes: 30 * 32 * 128 * 128 * 4},
			context: 262144, activeNear: 4.75e9,
		},
		{
			config:  "Qwen_Qwen3-Coder-Next",
			total:   79_674_391_296,
			want:    catalog.Attention{FullLayers: 12, FullKV: 1024, StateBytes: 36 * 32 * 128 * 128 * 4},
			context: 262144, activeNear: 3.9e9,
		},
		{
			config:  "google_gemma-4-26B-A4B-it",
			total:   25_805_933_872,
			want:    catalog.Attention{FullLayers: 5, FullKV: 2 * 2 * 512, SlidingLayers: 25, SlidingKV: 2 * 8 * 256, Window: 1024},
			context: 262144, activeNear: 4.4e9,
		},
		{
			// The last 18 of 42 layers reuse earlier layers' KV cache, and
			// full-attention layers use the wider global head.
			config:  "google_gemma-4-E4B-it",
			total:   7_996_156_490,
			want:    catalog.Attention{FullLayers: 4, FullKV: 2 * 2 * 512, SlidingLayers: 20, SlidingKV: 2 * 2 * 256, Window: 512},
			context: 131072, activeNear: 8e9,
		},
		{
			config:  "zai-org_GLM-4.7-Flash",
			total:   31_221_488_576,
			want:    catalog.Attention{FullLayers: 47, FullKV: 512 + 64},
			context: 202752, activeNear: 5.2e9,
		},
		{
			config:  "nvidia_NVIDIA-Nemotron-3-Nano-30B-A3B-BF16",
			total:   31_577_937_344,
			want:    catalog.Attention{FullLayers: 6, FullKV: 2 * 2 * 128, StateBytes: 23 * 64 * 64 * 128 * 4},
			context: 262144, activeNear: 3.6e9,
		},
		{
			config:  "nvidia_NVIDIA-Nemotron-3.5-Lightning-30B-A3B-BF16",
			total:   31_577_937_344,
			want:    catalog.Attention{FullLayers: 6, FullKV: 2 * 2 * 128, StateBytes: 23 * 64 * 64 * 128 * 4},
			context: 262144, activeNear: 3.6e9,
		},
		{
			config:  "openai_gpt-oss-20b",
			total:   20_914_757_184,
			want:    catalog.Attention{FullLayers: 12, FullKV: 2 * 8 * 64, SlidingLayers: 12, SlidingKV: 2 * 8 * 64, Window: 128},
			context: 131072, activeNear: 4.2e9,
		},
		{
			config:  "mistralai_Devstral-Small-2-24B-Instruct-2512",
			total:   24_011_361_280,
			want:    catalog.Attention{FullLayers: 40, FullKV: 2 * 8 * 128},
			context: 393216, activeNear: 24.0e9,
		},
	}
	for _, tt := range tests {
		t.Run(tt.config, func(t *testing.T) {
			arch, err := catalog.DeriveArchitecture(config(t, tt.config))
			if err != nil {
				t.Fatalf("DeriveArchitecture: %v", err)
			}
			if arch.Attention != tt.want {
				t.Errorf("Attention = %+v\n          want %+v", arch.Attention, tt.want)
			}
			if arch.ContextLength != tt.context {
				t.Errorf("ContextLength = %d, want %d", arch.ContextLength, tt.context)
			}
			active := float64(arch.ActiveParams(tt.total))
			if active < tt.activeNear*0.95 || active > tt.activeNear*1.05 {
				t.Errorf("ActiveParams = %.2fB, want about %.2fB", active/1e9, tt.activeNear/1e9)
			}
		})
	}
}

func TestDeriveFlagsUnknownSlidingSchemes(t *testing.T) {
	arch, err := catalog.DeriveArchitecture([]byte(`{"num_hidden_layers":43,"num_attention_heads":64,"num_key_value_heads":1,"head_dim":512,"sliding_window":128}`))
	if err != nil {
		t.Fatal(err)
	}
	if !arch.Attention.Approximate || arch.Attention.FullLayers != 43 {
		t.Errorf("Attention = %+v, want every layer treated as full and flagged approximate", arch.Attention)
	}
}

func TestDeriveRejectsIncompleteConfigs(t *testing.T) {
	for _, data := range []string{`{`, `{"hidden_size":4096}`} {
		if _, err := catalog.DeriveArchitecture([]byte(data)); err == nil {
			t.Errorf("DeriveArchitecture(%s) succeeded", data)
		}
	}
}

func TestKVBytes(t *testing.T) {
	gptOSS := catalog.Attention{FullLayers: 12, FullKV: 1024, SlidingLayers: 12, SlidingKV: 1024, Window: 128, StateBytes: 1000}
	tests := []struct {
		tokens int
		kv     catalog.KVType
		want   int64
	}{
		{0, catalog.KVF16, 1000},
		{100, catalog.KVF16, (12*1024*100+12*1024*100)*2 + 1000},
		{131072, catalog.KVF16, (12*1024*131072+12*1024*128)*2 + 1000},
		{131072, catalog.KVQ8, int64(float64(12*1024*131072+12*1024*128)*34/32) + 1000},
		{-5, catalog.KVF16, 1000},
	}
	for _, tt := range tests {
		if got := gptOSS.KVBytes(tt.tokens, tt.kv); got != tt.want {
			t.Errorf("KVBytes(%d, %s) = %d, want %d", tt.tokens, tt.kv, got, tt.want)
		}
	}
}

func TestBuildPrefersStatedActiveParams(t *testing.T) {
	arch, err := catalog.DeriveArchitecture(config(t, "Qwen_Qwen3.6-35B-A3B"))
	if err != nil {
		t.Fatal(err)
	}
	m := catalog.Build(catalog.Source{ID: "x", Name: "x", Tier: catalog.TierA, ActiveParams: 3e9, Summary: "s", Card: "c"},
		catalog.RepoInfo{Params: 35_951_822_704}, arch, "")
	if m.ActiveParams != 3e9 || m.Profile.Summary != "s" || m.Profile.Card != "c" {
		t.Errorf("Build = active %d, profile %+v", m.ActiveParams, m.Profile)
	}
}

func TestBuildAppliesCardOverrides(t *testing.T) {
	arch, err := catalog.DeriveArchitecture(config(t, "Qwen_Qwen3.6-35B-A3B"))
	if err != nil {
		t.Fatal(err)
	}
	info := catalog.RepoInfo{Params: 35_951_822_704, Released: "2026-04-01"}
	derived := catalog.Build(catalog.Source{ID: "x", Name: "x", Tier: catalog.TierA}, info, arch, "")
	if derived.StatedActive() != derived.ActiveParams || derived.Released != "2026-04-01" {
		t.Errorf("without overrides: stated %d, active %d, released %s", derived.StatedActive(), derived.ActiveParams, derived.Released)
	}

	m := catalog.Build(catalog.Source{ID: "x", Name: "x", Tier: catalog.TierA, StatedActiveParams: 3e9, ContextLength: 131_072, Released: "2026-04-16"}, info, arch, "")
	if m.StatedActive() != 3e9 || m.ActiveParams != derived.ActiveParams {
		t.Errorf("StatedActive = %d, ActiveParams = %d; want 3e9 shown and %d kept for speed", m.StatedActive(), m.ActiveParams, derived.ActiveParams)
	}
	if m.ContextLength != 131_072 || m.Released != "2026-04-16" {
		t.Errorf("ContextLength = %d, Released = %s; want the card's 131072 and 2026-04-16", m.ContextLength, m.Released)
	}
}

func TestBuild(t *testing.T) {
	arch, err := catalog.DeriveArchitecture(config(t, "openai_gpt-oss-20b"))
	if err != nil {
		t.Fatal(err)
	}
	src := catalog.Source{ID: "gpt-oss-20b", Name: "gpt-oss-20b", Tier: catalog.TierB, Quants: []catalog.Quant{{Name: "MXFP4", Bytes: 13_780_000_000}}}
	m := catalog.Build(src, catalog.RepoInfo{Params: 20_914_757_184, License: "apache-2.0"}, arch, "note")
	if len(m.Quants) != 1 || m.Quants[0].BitsPerWeight != 5.27 {
		t.Errorf("Quants = %+v, want MXFP4 at 5.27 bits per weight", m.Quants)
	}
	if !m.MoE() || m.TierNote != "note" || m.License != "apache-2.0" {
		t.Errorf("Build = %+v", m)
	}

	dense := catalog.Build(catalog.Source{ID: "d", Name: "d", Tier: catalog.TierC}, catalog.RepoInfo{Params: 8_000_000_000}, arch, "")
	if dense.Quants[0].Name != "Q8_0" || dense.Quants[0].Bytes != 8_500_000_000 || dense.Quants[len(dense.Quants)-1].Name != "Q2_K" {
		t.Errorf("GGUF ladder = %+v, want Q8_0 first at 8.5 GB and Q2_K last", dense.Quants)
	}
}

func TestDefaultCatalogIsValid(t *testing.T) {
	c, err := catalog.Default()
	if err != nil {
		t.Fatalf("embedded catalog: %v", err)
	}
	if len(c.Models) < 20 {
		t.Fatalf("embedded catalog has %d models", len(c.Models))
	}
	m, ok := c.ByOllamaTag("qwen3-coder:30b")
	if !ok || m.ID != "qwen3-coder-30b-a3b" {
		t.Errorf("ByOllamaTag(qwen3-coder:30b) = %v, %v", m.ID, ok)
	}
	if m, ok := c.ByOllamaTag("Qwen3.6:latest"); !ok || m.ID != "qwen3.6-35b-a3b" {
		t.Errorf("ByOllamaTag(Qwen3.6:latest) = %v, %v", m.ID, ok)
	}
	if _, ok := c.ByOllamaTag("hf.co/someone/unknown:Q4_K_M"); ok {
		t.Error("unknown tag matched a model")
	}
	for _, m := range c.Models {
		if m.TierNote == "" {
			t.Errorf("%s has no tier note", m.ID)
		}
		if m.Profile.Summary == "" || !strings.HasPrefix(m.Profile.Card, "https://huggingface.co/") {
			t.Errorf("%s has no reviewed profile with a card link", m.ID)
		}
		for _, line := range append(slices.Clone(m.Profile.GoodAt), m.Profile.WatchOut...) {
			if len([]rune(line)) > 60 {
				t.Errorf("%s: profile line longer than 60 characters: %q", m.ID, line)
			}
		}
	}
	if m, _ := c.ByOllamaTag("qwen3-coder:30b"); len(m.Profile.GoodAt) == 0 || len(m.Profile.WatchOut) == 0 {
		t.Errorf("qwen3-coder profile = %+v", m.Profile)
	}
}

func TestParseValidates(t *testing.T) {
	_, err := catalog.Parse([]byte(`{"models":[{"id":"x","name":"x","params":10,"activeParams":20,"tier":"Z"},{"id":"x","name":"x"}]}`))
	if err == nil {
		t.Fatal("Parse accepted an invalid catalog")
	}
	for _, want := range []string{"implausible parameters", "unknown tier", "no attention layers", "appears twice"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestTierRank(t *testing.T) {
	ranks := []int{catalog.TierA.Rank(), catalog.TierB.Rank(), catalog.TierC.Rank(), catalog.TierD.Rank(), catalog.Tier("?").Rank()}
	for i := 1; i < len(ranks); i++ {
		if ranks[i] <= ranks[i-1] {
			t.Errorf("tier ranks %v are not strictly increasing from A to D to unknown", ranks)
		}
	}
}

func TestParseSources(t *testing.T) {
	data, err := os.ReadFile("sources.json")
	if err != nil {
		t.Fatal(err)
	}
	s, err := catalog.ParseSources(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range s.Models {
		if s.TierNotes[src.Tier] == "" {
			t.Errorf("%s uses tier %q, which has no note", src.ID, src.Tier)
		}
		if _, err := catalog.ResolveBenchmarks(s.Benchmarks, src.Benchmarks); err != nil {
			t.Errorf("%s: %v", src.ID, err)
		}
	}
}
