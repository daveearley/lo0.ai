package match

import (
	"testing"

	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/machine"
	"lo0.ai/lo0/internal/runtimes"
	"lo0.ai/lo0/internal/usage"
)

const gib = int64(1) << 30

// denseModel has 1 MiB of f16 KV cache per 1,024 tokens: 16 layers of 32
// values each.
func denseModel() catalog.Model {
	return catalog.Model{
		ID: "dense", Name: "Dense", Params: 10e9, ActiveParams: 10e9, ContextLength: 1 << 20,
		Attention: catalog.Attention{FullLayers: 16, FullKV: 32},
		Quants: []catalog.Quant{
			{Name: "Q8_0", Bytes: 10 * gib, BitsPerWeight: 8.5},
			{Name: "Q4_K_M", Bytes: 6 * gib, BitsPerWeight: 4.85},
			{Name: "Q2_K", Bytes: 4 * gib, BitsPerWeight: 3},
		},
		Tier: catalog.TierA,
	}
}

func peaks(values ...int) usage.Context { return usage.Context{Peaks: values} }

func TestOptionFindsLargestContext(t *testing.T) {
	m := denseModel()
	f := fitter{budget: 6*gib + RuntimeOverhead + 256<<20, speeds: bandwidthOnly(300), kvTypes: bothCaches, context: peaks(100_000, 200_000, 300_000), reference: 100_000}
	o := f.option(m, m.Quants[1], catalog.KVF16, 0)
	if o.MaxContext != 256<<10 {
		t.Fatalf("MaxContext = %d, want %d (256 MiB of KV at 1 KiB per token)", o.MaxContext, 256<<10)
	}
	if o.Coverage != 2.0/3 {
		t.Errorf("Coverage = %v, want 2/3", o.Coverage)
	}
	if o.MemoryBytes > f.budget {
		t.Errorf("MemoryBytes %d exceeds budget %d", o.MemoryBytes, f.budget)
	}
	want := o.Curve.WriteAt(cacheBytes(m, catalog.KVF16, 100_000))
	if o.TokensPerSecond != want || o.TokensPerSecond <= 0 || o.ReadTokensPerSecond <= 0 || o.Curve.Source != SourceBandwidth {
		t.Errorf("speed = %v write, %v read from %s, want the curve at the reference context", o.TokensPerSecond, o.ReadTokensPerSecond, o.Curve.Source)
	}
}

func TestOptionCapsAtModelContext(t *testing.T) {
	m := denseModel()
	m.ContextLength = 64 << 10
	o := fitter{budget: 100 * gib, speeds: bandwidthOnly(300), kvTypes: bothCaches, context: peaks(1000)}.option(m, m.Quants[0], catalog.KVF16, 0)
	if o.MaxContext != 64<<10 {
		t.Errorf("MaxContext = %d, want the model limit %d", o.MaxContext, 64<<10)
	}
}

func TestOptionTooSmall(t *testing.T) {
	m := denseModel()
	o := fitter{budget: 5 * gib, speeds: bandwidthOnly(300), kvTypes: bothCaches}.option(m, m.Quants[1], catalog.KVF16, 0)
	if o.MaxContext != 0 || o.Coverage != 0 || o.MemoryBytes <= 5*gib {
		t.Errorf("option = %+v, want no context and the memory it would need", o)
	}
}

func TestBestBalancesQualityAndSpeed(t *testing.T) {
	m := denseModel()
	sessions := peaks(10_000, 20_000, 30_000, 40_000)

	fast := fitter{budget: 12 * gib, speeds: bandwidthOnly(2000), kvTypes: bothCaches, context: sessions, reference: 20_000}
	if o, ok := best(fast.options(m)); !ok || o.Quant.Name != "Q8_0" || o.KV != catalog.KVF16 {
		t.Errorf("best on fast memory = %+v, want Q8_0 with f16 KV, since every option is quick enough", o)
	}

	slow := fitter{budget: 12 * gib, speeds: bandwidthOnly(300), kvTypes: bothCaches, context: sessions, reference: 20_000}
	if o, ok := best(slow.options(m)); !ok || o.Quant.Name != "Q4_K_M" {
		t.Errorf("best on slow memory = %+v, want Q4_K_M, which is much faster for little quality", o)
	}

	tight := fitter{budget: 10*gib + RuntimeOverhead + 40<<20, speeds: bandwidthOnly(2000), kvTypes: bothCaches, context: peaks(30_000, 60_000, 90_000, 120_000)}
	if o, ok := best(tight.options(m)); !ok || o.Quant.Name != "Q4_K_M" {
		t.Errorf("best = %+v, want Q4_K_M when Q8_0 leaves sessions out", o)
	}
}

func TestQuality(t *testing.T) {
	tests := []struct {
		bits float64
		kv   catalog.KVType
		want float64
	}{
		{8.5, catalog.KVF16, 1},
		{6.5, catalog.KVF16, 1},
		{4.8, catalog.KVF16, 0.965},
		{3.0, catalog.KVF16, 0.7},
		{2.0, catalog.KVF16, 0.5},
		{8.5, catalog.KVQ8, 0.99},
	}
	for _, tt := range tests {
		if got := quality(tt.bits, tt.kv); got < tt.want-1e-9 || got > tt.want+1e-9 {
			t.Errorf("quality(%v, %s) = %v, want %v", tt.bits, tt.kv, got, tt.want)
		}
	}
	if q4, q5 := quality(4.85, catalog.KVF16), quality(5.69, catalog.KVF16); !(q4 < q5 && q5 < 1) {
		t.Errorf("quality is not monotonic: Q4_K_M %v, Q5_K_M %v", q4, q5)
	}
}

func TestBestAvoidsLowBitQuantsUnlessForced(t *testing.T) {
	m := denseModel()
	f := fitter{budget: 4*gib + RuntimeOverhead + 64<<20, speeds: bandwidthOnly(300), kvTypes: bothCaches, context: peaks(20_000)}
	o, ok := best(f.options(m))
	if !ok || o.Quant.Name != "Q2_K" {
		t.Errorf("best = %+v, want Q2_K as the only option that fits", o)
	}
}

func TestBestNothingFits(t *testing.T) {
	if _, ok := best((fitter{budget: 2 * gib, speeds: bandwidthOnly(300), kvTypes: bothCaches}).options(denseModel())); ok {
		t.Error("best found an option in 2 GiB")
	}
}

func TestInstalledOptionUsesRealSize(t *testing.T) {
	m := denseModel()
	f := fitter{budget: 16 * gib, speeds: bandwidthOnly(300), kvTypes: bothCaches, context: peaks(50_000)}
	o := f.installedOption(m, runtimes.Model{Name: "dense:latest", Quantization: "q5_k_m", SizeBytes: uint64(7 * gib)})
	if o.Quant.Name != "Q5_K_M" || o.Quant.Bytes != 7*gib || o.Coverage != 1 {
		t.Errorf("installed option = %+v", o)
	}
	if bits := o.Quant.BitsPerWeight; bits < 6 || bits > 6.1 {
		t.Errorf("BitsPerWeight = %v, want about 6.01", bits)
	}
}

func TestMemoryBandwidth(t *testing.T) {
	apple := func(name string, cores int) machine.Profile {
		return machine.Profile{GPUs: []machine.GPU{{Name: name, Vendor: machine.VendorApple, Cores: cores, Unified: true}}}
	}
	nvidia := func(names ...string) machine.Profile {
		var p machine.Profile
		for _, n := range names {
			p.GPUs = append(p.GPUs, machine.GPU{Name: n, Vendor: machine.VendorNVIDIA})
		}
		return p
	}
	tests := []struct {
		name      string
		profile   machine.Profile
		gbps      float64
		estimated bool
	}{
		{"M5 Pro", apple("Apple M5 Pro", 20), 307, false},
		{"M5", apple("Apple M5", 10), 153, false},
		{"M5 Ultra is not an M5", apple("Apple M5 Ultra", 80), 1200, false},
		{"M6", apple("Apple M6", 8), 170, false},
		{"binned M5 Max", apple("Apple M5 Max", 32), 460, false},
		{"full M5 Max", apple("Apple M5 Max", 40), 614, false},
		{"binned M3 Max", apple("Apple M3 Max", 30), 300, false},
		{"future Apple chip", apple("Apple M9 Hyper", 80), fallbackUnifiedGBps, true},
		{"4070 Ti SUPER is not a 4070", nvidia("NVIDIA GeForce RTX 4070 Ti SUPER"), 672, false},
		{"plain 4070", nvidia("NVIDIA GeForce RTX 4070"), 504, false},
		{"3060 Ti is not a 3060", nvidia("NVIDIA GeForce RTX 3060 Ti"), 448, false},
		{"laptop card is not the desktop one", nvidia("NVIDIA GeForce RTX 4090 Laptop GPU"), fallbackDiscreteGBps, true},
		{"A100 on a word boundary", nvidia("NVIDIA A100-SXM4-80GB"), 2039, false},
		{"H100 PCIe is not the SXM part", nvidia("NVIDIA H100 PCIe"), 2000, false},
		{"H100 SXM", nvidia("NVIDIA H100 80GB HBM3"), 3350, false},
		{"A100 40GB", nvidia("NVIDIA A100-PCIE-40GB"), 1555, false},
		{"A100 80GB PCIe", nvidia("NVIDIA A100 80GB PCIe"), 1935, false},
		{"slowest card sets the pace", nvidia("NVIDIA GeForce RTX 4090", "NVIDIA GeForce RTX 3090"), 936, false},
		{"unknown card", nvidia("NVIDIA Quadro Mystery"), fallbackDiscreteGBps, true},
		{"CPU only", machine.Profile{}, fallbackCPUGBps, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bw := MemoryBandwidth(tt.profile)
			if bw.GBps != tt.gbps || bw.Estimated != tt.estimated {
				t.Errorf("MemoryBandwidth = %+v, want %v GB/s (estimated %v)", bw, tt.gbps, tt.estimated)
			}
		})
	}
}

// bandwidthOnly is the speed evidence of a machine known only by its
// bandwidth, in GB/s.
func bandwidthOnly(gbps float64) speeds {
	return speeds{rates: machineRates(machine.Profile{}, Bandwidth{GBps: gbps})}
}

var bothCaches = []catalog.KVType{catalog.KVF16, catalog.KVQ8}
