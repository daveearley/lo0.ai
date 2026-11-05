package match

import (
	"math"
	"strconv"
	"testing"
	"time"

	"lo0.ai/lo0/internal/bench"
	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/machine"
	"lo0.ai/lo0/internal/runtimes"
)

func m5Pro() machine.Profile {
	return machine.Profile{GPUs: []machine.GPU{{Name: "Apple M5 Pro", Vendor: machine.VendorApple, Cores: 20, Unified: true}}}
}

func catalogBuild(t *testing.T, id, quant string) (*catalog.Catalog, catalog.Model, catalog.Quant) {
	t.Helper()
	cat, err := catalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range cat.Models {
		if m.ID != id {
			continue
		}
		for _, q := range m.Quants {
			if q.Name == quant {
				return cat, m, q
			}
		}
	}
	t.Fatalf("%s at %s is not in the catalogue", id, quant)
	return nil, catalog.Model{}, catalog.Quant{}
}

func closeTo(t *testing.T, what string, got, want, share float64) {
	t.Helper()
	if math.Abs(got-want)/want > share {
		t.Errorf("%s = %.1f, measured %.1f: more than %.0f%% off", what, got, want, share*100)
	}
}

// TestPredictionsMatchMeasurements holds the speed model to what lo0
// measured on a 20-core M5 Pro with Ollama 0.34.4 on 5 November 2026, with
// a fresh prompt each time and a 16-bit cache (docs/design/speed.md). The
// prediction uses only llama.cpp's published M5 Pro result and the model's
// architecture.
func TestPredictionsMatchMeasurements(t *testing.T) {
	r := machineRates(m5Pro(), Bandwidth{GBps: 307})
	if r.source != SourcePublished || r.writeEff < 0.8 || r.writeEff > 0.85 {
		t.Fatalf("rates = %+v, want llama.cpp's M5 Pro result, about 83%% of bandwidth", r)
	}
	for _, tt := range []struct {
		model, quant  string
		context       int
		write, readUp float64
	}{
		{"qwen3.6-35b-a3b", "Q4_K_M", 7020, 73.3, 1423},
		{"qwen3.6-35b-a3b", "Q4_K_M", 38220, 55.3, 774},
		{"qwen3.6-35b-a3b", "Q4_K_M", 112821, 31.2, 294},
		{"qwen3-coder-30b-a3b", "Q4_K_M", 6865, 56.2, 725},
		{"qwen3-coder-30b-a3b", "Q4_K_M", 37416, 22.4, 199},
	} {
		_, m, q := catalogBuild(t, tt.model, tt.quant)
		c := r.predict(m, q)
		at := cacheBytes(m, catalog.KVF16, tt.context)
		closeTo(t, tt.model+" writing at "+strconv.Itoa(tt.context), c.WriteAt(at), tt.write, 0.2)
		closeTo(t, tt.model+" reading "+strconv.Itoa(tt.context)+" cold", c.ReadCold(at), tt.readUp, 0.35)
	}
}

func TestMeasuredCurveFitsItsPoints(t *testing.T) {
	cat, m, q := catalogBuild(t, "qwen3.6-35b-a3b", "Q4_K_M")
	measured := []bench.Result{{
		Model: m.ID, Quant: q.Name, KV: "f16", Runtime: runtimes.Ollama, Device: Device(m5Pro()),
		At: time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC),
		Points: []bench.Point{
			{Context: 7020, ReadTokensPerSecond: 1423, WriteTokensPerSecond: 73.3},
			{Context: 112821, ReadTokensPerSecond: 294, WriteTokensPerSecond: 31.2},
		},
	}}
	s := newSpeeds(m5Pro(), Bandwidth{GBps: 307}, measured, cat)
	c := s.curve(m, q, catalog.KVF16)
	if c.Source != SourceMeasured || c.MeasuredAt.IsZero() {
		t.Fatalf("curve = %+v, want the measurement", c)
	}
	for _, p := range measured[0].Points {
		at := cacheBytes(m, catalog.KVF16, p.Context)
		closeTo(t, "fitted writing", c.WriteAt(at), p.WriteTokensPerSecond, 0.001)
		closeTo(t, "fitted cold reading", c.ReadCold(at), p.ReadTokensPerSecond, 0.001)
	}

	// Another model on the same machine is calibrated with it.
	_, other, oq := catalogBuild(t, "qwen3-coder-30b-a3b", "Q4_K_M")
	if got := s.curve(other, oq, catalog.KVF16); got.Source != SourceMachine {
		t.Errorf("other model's source = %s, want this machine's calibration", got.Source)
	}

	// A measurement from another machine is not used.
	elsewhere := newSpeeds(machine.Profile{GPUs: []machine.GPU{{Name: "Apple M4 Pro", Vendor: machine.VendorApple, Cores: 20}}}, Bandwidth{GBps: 273}, measured, cat)
	if got := elsewhere.curve(m, q, catalog.KVF16); got.Source != SourcePublished {
		t.Errorf("source on another machine = %s, want its own published result", got.Source)
	}
}

func TestPublishedFor(t *testing.T) {
	for _, tt := range []struct {
		gpu   machine.GPU
		want  string
		cores int
		ok    bool
	}{
		{machine.GPU{Name: "Apple M4 Pro", Vendor: machine.VendorApple, Cores: 20}, "M4 Pro", 20, true},
		{machine.GPU{Name: "Apple M4 Pro", Vendor: machine.VendorApple, Cores: 14}, "M4 Pro", 16, true},
		{machine.GPU{Name: "Apple M4", Vendor: machine.VendorApple, Cores: 10}, "M4", 10, true},
		{machine.GPU{Name: "Apple M5", Vendor: machine.VendorApple, Cores: 10}, "M5", 10, true},
		{machine.GPU{Name: "Apple M5 Ultra", Vendor: machine.VendorApple, Cores: 80}, "M5 Ultra", 80, true},
		{machine.GPU{Name: "Apple M7", Vendor: machine.VendorApple, Cores: 10}, "", 0, false},
		{machine.GPU{Name: "NVIDIA GeForce RTX 5090 Laptop GPU", Vendor: machine.VendorNVIDIA}, "RTX 5090 Laptop", 0, true},
		{machine.GPU{Name: "NVIDIA GeForce RTX 4090 D", Vendor: machine.VendorNVIDIA}, "", 0, false},
		{machine.GPU{Name: "NVIDIA RTX A1000", Vendor: machine.VendorNVIDIA}, "", 0, false},
		{machine.GPU{Name: "NVIDIA GeForce RTX 4090", Vendor: machine.VendorNVIDIA}, "RTX 4090", 0, true},
		{machine.GPU{Name: "NVIDIA GeForce RTX 4060 Ti", Vendor: machine.VendorNVIDIA}, "RTX 4060 Ti", 0, true},
	} {
		got, _, ok := publishedFor(machine.Profile{GPUs: []machine.GPU{tt.gpu}})
		if ok != tt.ok || got.match != tt.want || got.gpuCores != tt.cores {
			t.Errorf("%s %d-core: got %+v %v, want %s %d", tt.gpu.Name, tt.gpu.Cores, got, ok, tt.want, tt.cores)
		}
	}
}

func TestCacheTypesFollowTheRuntime(t *testing.T) {
	ollama := []runtimes.Runtime{{ID: runtimes.Ollama, Installed: true}, {ID: runtimes.LlamaCpp, Installed: true}}
	if got := cacheTypes(ollama); len(got) != 1 || got[0] != catalog.KVF16 {
		t.Errorf("with Ollama = %v, want its 16-bit default only", got)
	}
	if got := cacheTypes(nil); len(got) != 1 {
		t.Errorf("with nothing installed = %v, want Ollama's default, as lo0 would set Ollama up", got)
	}
	if got := cacheTypes([]runtimes.Runtime{{ID: runtimes.LlamaCpp, Installed: true}}); len(got) != 2 {
		t.Errorf("with llama.cpp alone = %v, want both precisions", got)
	}
}

func TestCheckMeasurementRefusesABusyRun(t *testing.T) {
	cat, m, q := catalogBuild(t, "qwen3.6-35b-a3b", "Q4_K_M")
	run := func(points ...bench.Point) bench.Result {
		return bench.Result{Model: m.ID, Quant: q.Name, KV: "f16", Points: points}
	}
	// This morning's clean measurements, and the run taken while macOS was
	// swapping, both on the same M5 Pro.
	clean := run(bench.Point{Context: 7020, ReadTokensPerSecond: 1423, WriteTokensPerSecond: 73.3},
		bench.Point{Context: 38220, ReadTokensPerSecond: 774, WriteTokensPerSecond: 55.3},
		bench.Point{Context: 112821, ReadTokensPerSecond: 294, WriteTokensPerSecond: 31.2})
	busy := run(bench.Point{Context: 8174, ReadTokensPerSecond: 1449, WriteTokensPerSecond: 77.2},
		bench.Point{Context: 33278, ReadTokensPerSecond: 897, WriteTokensPerSecond: 61.2},
		bench.Point{Context: 97572, ReadTokensPerSecond: 216, WriteTokensPerSecond: 12.9})
	if err := CheckMeasurement(cat, clean); err != nil {
		t.Errorf("clean run refused: %v", err)
	}
	if err := CheckMeasurement(cat, busy); err == nil {
		t.Error("the run taken while swapping was accepted")
	}
}
