package match

import (
	"testing"

	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/machine"
	"lo0.ai/lo0/internal/usage"
)

// pc is a desktop with one discrete card and ram GiB of system memory.
func pc(card string, vram, ram int64) machine.Profile {
	return machine.Profile{
		Memory: machine.Memory{TotalBytes: uint64(ram * gib)},
		GPUs:   []machine.GPU{{Name: "NVIDIA GeForce " + card, Vendor: machine.VendorNVIDIA, VRAMBytes: uint64(vram * gib)}},
	}
}

func pcFitter(p machine.Profile, cat *catalog.Catalog, sessions usage.Context) fitter {
	return fitter{
		budget: int64(p.ModelMemory().Bytes), host: int64(p.ExpertMemory()),
		speeds: newSpeeds(p, MemoryBandwidth(p), nil, cat), kvTypes: []catalog.KVType{catalog.KVF16},
		context: sessions, reference: 16 << 10,
	}
}

// TestOffloadedSpeedMatchesAMeasurement: Qwen3.6-35B-A3B Q4_K_M on an RTX
// 5080 with DDR5-5600 and 15 layers' experts in RAM wrote 79 tok/s at 4k.
// https://github.com/Andyyyy64/whichllm#example
func TestOffloadedSpeedMatchesAMeasurement(t *testing.T) {
	cat, m, q := catalogBuild(t, "qwen3.6-35b-a3b", "Q4_K_M")
	p := pc("RTX 5080", 16, 64)
	f := pcFitter(p, cat, peaks(4096))
	o := f.option(m, q, catalog.KVF16, 15)
	if o.Offload.Leading != 15 || o.Offload.Bytes <= 0 {
		t.Fatalf("Offload = %+v, want the first 15 layers' experts", o.Offload)
	}
	got := o.Curve.WriteAt(cacheBytes(m, catalog.KVF16, 4096))
	// lo0 assumes DDR4, so it may come in under the DDR5 run.
	if got < 79*0.7 || got > 79*1.1 {
		t.Errorf("writing at 4k = %.0f tok/s, want close to the measured 79 and not above it", got)
	}
	t.Logf("writing at 4k = %.0f tok/s, reading %.0f", got, o.ReadTokensPerSecond)
	onGPU := f.speeds.curve(m, q, catalog.KVF16, Offload{}).ReadAt(cacheBytes(m, catalog.KVF16, f.reference))
	if o.ReadTokensPerSecond >= onGPU/4 {
		t.Errorf("reading = %.0f tok/s with experts in system memory against %.0f on the GPU; copying them for each batch should cost far more", o.ReadTokensPerSecond, onGPU)
	}
}

func TestOffloadFitsAModelLargerThanTheCard(t *testing.T) {
	cat, m, _ := catalogBuild(t, "qwen3.6-35b-a3b", "Q4_K_M")
	p := pc("RTX 4060", 8, 32)
	f := pcFitter(p, cat, peaks(20_000, 40_000, 60_000, 90_000))
	options := f.options(m)
	top, ok := best(options)
	if !ok || !top.Offloads() {
		t.Fatalf("best = %+v, want experts in system memory: no build fits an 8 GB card with a useful window", top)
	}
	if top.MemoryBytes > f.budget || top.Offload.Bytes > f.host {
		t.Errorf("best needs %d on the GPU of %d and %d in system memory of %d", top.MemoryBytes, f.budget, top.Offload.Bytes, f.host)
	}
	if top.Coverage < 1 || top.TokensPerSecond < 8 {
		t.Errorf("best covers %.0f%% at %.0f tok/s, want every session at a usable speed", top.Coverage*100, top.TokensPerSecond)
	}
	if fewer := f.option(m, top.Quant, top.KV, top.Offload.Layers-1); fewer.MaxContext >= top.MaxContext {
		t.Errorf("%d layers reach %d tokens as %d do; offload moved more than it needed", top.Offload.Layers-1, fewer.MaxContext, top.Offload.Layers)
	}
	for _, o := range options {
		if !o.Offloads() {
			continue
		}
		if o.Offload.Leading != o.Offload.Layers {
			t.Errorf("Leading = %d for %d layers; every Qwen3.6 layer has experts", o.Offload.Leading, o.Offload.Layers)
		}
	}
}

func TestNoOffloadWhenItGainsNothing(t *testing.T) {
	cat, m, _ := catalogBuild(t, "qwen3.6-35b-a3b", "Q4_K_M")
	sessions := peaks(20_000, 40_000)
	for name, p := range map[string]machine.Profile{
		"unified memory":       {Memory: machine.Memory{TotalBytes: uint64(64 * gib), Unified: true}, GPUs: []machine.GPU{{Name: "Apple M5 Pro", Vendor: machine.VendorApple, Cores: 20, Unified: true}}},
		"no GPU":               {Memory: machine.Memory{TotalBytes: uint64(64 * gib)}},
		"a card that holds it": pc("RTX 5090", 32, 64),
	} {
		t.Run(name, func(t *testing.T) {
			for _, o := range pcFitter(p, cat, sessions).options(m) {
				// Q8_0 is larger than the 5090.
				if o.Offloads() && o.Quant.BitsPerWeight < 8 {
					t.Errorf("option %s %s keeps %d layers' experts in system memory", o.Quant.Name, o.KV, o.Offload.Layers)
				}
			}
		})
	}
}

func TestDenseModelsNeverOffload(t *testing.T) {
	m := denseModel()
	p := pc("RTX 4060", 8, 64)
	f := pcFitter(p, nil, peaks(20_000))
	for _, o := range f.options(m) {
		if o.Offloads() {
			t.Errorf("dense option %s offloads", o.Quant.Name)
		}
	}
}

func TestOffloadRespectsSystemMemory(t *testing.T) {
	cat, m, _ := catalogBuild(t, "qwen3-coder-next", "Q4_K_M")
	small := pc("RTX 4060", 8, 16)
	f := pcFitter(small, cat, peaks(20_000))
	for _, o := range f.options(m) {
		if o.Offload.Bytes > f.host {
			t.Errorf("%s keeps %d bytes in %d of system memory", o.Quant.Name, o.Offload.Bytes, f.host)
		}
	}
}
