package match

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/machine"
	"lo0.ai/lo0/internal/scan"
	"lo0.ai/lo0/internal/usage"
)

var update = flag.Bool("update", false, "rewrite golden files")

const (
	mib = uint64(1) << 20
	gb  = uint64(1) << 30
)

// archetype is a machine people commonly run local models on.
type archetype struct {
	name    string
	machine machine.Profile
	// midsize requires a 27B dense or 30B MoE model in the top three.
	midsize bool
}

func apple(chip string, cores int, memory uint64) machine.Profile {
	return machine.Profile{
		OS:     machine.OS{GOOS: "darwin", Arch: "arm64"},
		CPU:    machine.CPU{Model: "Apple " + chip},
		Memory: machine.Memory{TotalBytes: memory, Unified: true},
		GPUs:   []machine.GPU{{Name: "Apple " + chip, Vendor: machine.VendorApple, Cores: cores, API: "Metal 4", Unified: true}},
	}
}

// desktop is a PC with discrete cards, their VRAM as the driver reports it.
func desktop(memory uint64, vendor machine.Vendor, card string, vram uint64, cards int) machine.Profile {
	api := "CUDA"
	if vendor == machine.VendorAMD {
		api = "ROCm"
	}
	p := machine.Profile{
		OS:     machine.OS{GOOS: "linux", Arch: "amd64"},
		CPU:    machine.CPU{Model: "AMD Ryzen 9 7950X", PhysicalCores: 16, LogicalCores: 32},
		Memory: machine.Memory{TotalBytes: memory},
	}
	for range cards {
		p.GPUs = append(p.GPUs, machine.GPU{Name: card, Vendor: vendor, VRAMBytes: vram, API: api})
	}
	return p
}

func cpuOnly(memory uint64, gpus ...machine.GPU) machine.Profile {
	return machine.Profile{
		OS:     machine.OS{GOOS: "linux", Arch: "amd64"},
		CPU:    machine.CPU{Model: "Intel Core i7-12700", PhysicalCores: 12, LogicalCores: 20},
		Memory: machine.Memory{TotalBytes: memory},
		GPUs:   gpus,
	}
}

func archetypes() []archetype {
	return []archetype{
		{name: "m1-8gb", machine: apple("M1", 8, 8*gb)},
		{name: "m2-16gb", machine: apple("M2", 10, 16*gb)},
		{name: "m4-24gb", machine: apple("M4", 10, 24*gb)},
		{name: "m3-pro-36gb", machine: apple("M3 Pro", 18, 36*gb), midsize: true},
		{name: "m4-pro-48gb", machine: apple("M4 Pro", 20, 48*gb)},
		{name: "m4-max-64gb", machine: apple("M4 Max", 40, 64*gb)},
		{name: "m5-pro-64gb", machine: apple("M5 Pro", 20, 64*gb)},
		{name: "m5-max-128gb", machine: apple("M5 Max", 40, 128*gb)},
		{name: "rtx4060-8gb-ram16", machine: desktop(16*gb, machine.VendorNVIDIA, "NVIDIA GeForce RTX 4060", 8188*mib, 1)},
		{name: "rtx4060ti-16gb-ram32", machine: desktop(32*gb, machine.VendorNVIDIA, "NVIDIA GeForce RTX 4060 Ti", 16380*mib, 1)},
		{name: "rtx5080-16gb-ram32", machine: desktop(32*gb, machine.VendorNVIDIA, "NVIDIA GeForce RTX 5080", 16303*mib, 1)},
		{name: "rtx3090-24gb-ram32", machine: desktop(32*gb, machine.VendorNVIDIA, "NVIDIA GeForce RTX 3090", 24576*mib, 1), midsize: true},
		{name: "rtx4090-24gb-ram16", machine: desktop(16*gb, machine.VendorNVIDIA, "NVIDIA GeForce RTX 4090", 24564*mib, 1), midsize: true},
		{name: "rtx4090-24gb-ram64", machine: desktop(64*gb, machine.VendorNVIDIA, "NVIDIA GeForce RTX 4090", 24564*mib, 1), midsize: true},
		{name: "rtx5090-32gb-ram64", machine: desktop(64*gb, machine.VendorNVIDIA, "NVIDIA GeForce RTX 5090", 32607*mib, 1)},
		{name: "2x-rtx3090-48gb-ram64", machine: desktop(64*gb, machine.VendorNVIDIA, "NVIDIA GeForce RTX 3090", 24576*mib, 2)},
		{name: "rx7900xtx-24gb-ram32", machine: desktop(32*gb, machine.VendorAMD, "AMD Radeon RX 7900 XTX", 24560*mib, 1), midsize: true},
		{name: "cpu-16gb", machine: cpuOnly(16 * gb)},
		{name: "cpu-64gb-igpu", machine: cpuOnly(64*gb, machine.GPU{Name: "Intel UHD Graphics 770", Vendor: machine.VendorIntel, VRAMBytes: 128 * mib})},
	}
}

// sessions is a history shape: the peak context of each session.
type sessions struct {
	name  string
	peaks []int
}

// histories are short sessions around 30k tokens and long ones past 200k.
func histories() []sessions {
	return []sessions{
		{name: "short", peaks: []int{
			8_000, 11_000, 14_000, 17_000, 20_000, 22_000, 24_000, 26_000, 28_000, 30_000,
			31_000, 33_000, 35_000, 37_000, 40_000, 43_000, 46_000, 50_000, 56_000, 64_000,
		}},
		{name: "long", peaks: []int{
			40_000, 60_000, 80_000, 95_000, 110_000, 120_000, 130_000, 140_000, 150_000, 160_000,
			170_000, 180_000, 190_000, 200_000, 210_000, 220_000, 240_000, 260_000, 290_000, 320_000,
		}},
	}
}

func historyOf(peaks []int) usage.Profile {
	peaks = slices.Sorted(slices.Values(peaks))
	return usage.Profile{
		Sessions: len(peaks),
		Context:  usage.Context{P50: peaks[len(peaks)/2], P90: peaks[len(peaks)*9/10], Peaks: peaks},
	}
}

func embeddedCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	cat, err := catalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

// TestArchetypes checks each archetype's answer is sensible without pinning
// the order.
func TestArchetypes(t *testing.T) {
	cat := embeddedCatalog(t)
	for _, a := range archetypes() {
		for _, h := range histories() {
			t.Run(a.name+"/"+h.name, func(t *testing.T) {
				r := Match(scan.Result{Machine: a.machine, Usage: historyOf(h.peaks)}, cat, nil)
				checkArchetype(t, a, r)
			})
		}
	}
}

func checkArchetype(t *testing.T, a archetype, r Result) {
	t.Helper()
	top, ok := r.Best()
	if !ok {
		t.Fatalf("nothing fits in %s; every archetype should get a recommendation", format.Bytes(r.Budget.Bytes))
	}
	for _, c := range r.Candidates {
		if c.Model.Tier == catalog.TierD {
			t.Errorf("%s is tier D, which has no usable tool calling", c.Model.ID)
		}
		if c.Best.MemoryBytes > int64(r.Budget.Bytes) {
			t.Errorf("%s at %s needs %s, more than the %s budget", c.Model.ID, c.Best.Quant.Name,
				format.Bytes(uint64(c.Best.MemoryBytes)), format.Bytes(r.Budget.Bytes))
		}
		if c.Best.MaxContext < minUsefulContext {
			t.Errorf("%s has a %s window, below the %s an agent needs", c.Model.ID,
				format.Tokens(c.Best.MaxContext), format.Tokens(minUsefulContext))
		}
		if off := c.Best.Offload; off.Layers > 0 {
			if r.Budget.Source != machine.MemoryVRAM {
				t.Errorf("%s keeps experts in system memory on a %s machine", c.Model.ID, r.Budget.Source)
			}
			if off.Bytes > int64(r.ExpertMemory) || off.Leading < off.Layers {
				t.Errorf("%s keeps %s of experts in %s of system memory, --n-cpu-moe %d for %d layers", c.Model.ID,
					format.Bytes(uint64(off.Bytes)), format.Bytes(r.ExpertMemory), off.Leading, off.Layers)
			}
		}
	}
	if r.Budget.Source == machine.MemoryCPU && r.Verdict.Level == LevelReady {
		t.Errorf("a machine without a GPU for models was called ready")
	}
	if r.Verdict.Level == LevelReady && top.Model.Tier != catalog.TierA {
		t.Errorf("verdict is ready with a tier %s top pick", top.Model.Tier)
	}
	if r.Verdict.Level == LevelReady && top.Best.Quant.BitsPerWeight < minQualityBits {
		t.Errorf("verdict is ready with the top pick at %s, below %.1f bits a weight", top.Best.Quant.Name, minQualityBits)
	}
	if slow := slowerThanNeeded(r); slow != "" {
		t.Error(slow)
	}
	if a.midsize && !slices.ContainsFunc(r.Candidates[:min(3, len(r.Candidates))], midsizeModel) {
		t.Errorf("top three %v include no 27B-class dense or 30B-class mixture-of-experts model", topIDs(r, 3))
	}
}

// slowerThanNeeded describes a top pick below minSpeed when another reaches
// it with at least its coverage, or returns "".
func slowerThanNeeded(r Result) string {
	top, _ := r.Best()
	if top.Best.TokensPerSecond >= minSpeed {
		return ""
	}
	for _, c := range r.Candidates[1:] {
		if c.Best.TokensPerSecond >= minSpeed && c.Best.Coverage >= top.Best.Coverage {
			return fmt.Sprintf("top pick %s writes %.1f tok/s, below %d, though %s writes %.1f and covers as many sessions",
				top.Model.ID, top.Best.TokensPerSecond, minSpeed, c.Model.ID, c.Best.TokensPerSecond)
		}
	}
	return ""
}

// midsizeModel reports whether c is a 27B dense or 30B MoE model.
func midsizeModel(c Candidate) bool {
	return c.Model.Params >= 24e9 && c.Model.Params <= 37e9
}

func topIDs(r Result, n int) []string {
	var out []string
	for _, c := range r.Candidates[:min(n, len(r.Candidates))] {
		out = append(out, c.Model.ID)
	}
	return out
}

func TestMoreSystemMemoryNeverHurts(t *testing.T) {
	cat := embeddedCatalog(t)
	cards := []struct {
		name string
		vram uint64
	}{
		{"NVIDIA GeForce RTX 4060", 8188 * mib},
		{"NVIDIA GeForce RTX 5080", 16303 * mib},
		{"NVIDIA GeForce RTX 4090", 24564 * mib},
	}
	for _, card := range cards {
		for _, h := range histories() {
			t.Run(card.name+"/"+h.name, func(t *testing.T) {
				prev := 0.0
				for _, ram := range []uint64{16 * gb, 32 * gb, 64 * gb} {
					p := desktop(ram, machine.VendorNVIDIA, card.name, card.vram, 1)
					r := Match(scan.Result{Machine: p, Usage: historyOf(h.peaks)}, cat, nil)
					top, ok := r.Best()
					if !ok {
						t.Fatalf("nothing fits with %s of RAM", format.Bytes(ram))
					}
					if top.Score < prev {
						t.Errorf("with %s of RAM the top score fell to %.3f from %.3f", format.Bytes(ram), top.Score, prev)
					}
					prev = top.Score
				}
			})
		}
	}
}

// TestArchetypeTopPicks records each archetype's top three in a golden file.
func TestArchetypeTopPicks(t *testing.T) {
	cat := embeddedCatalog(t)
	var b strings.Builder
	b.WriteString("Top three picks per archetype: model, quant, KV cache, window, tok/s, coverage, expert layers in RAM.\n")
	for _, a := range archetypes() {
		for _, h := range histories() {
			r := Match(scan.Result{Machine: a.machine, Usage: historyOf(h.peaks)}, cat, nil)
			fmt.Fprintf(&b, "\n%s, %s sessions: %s %s, %s\n", a.name, h.name,
				format.Bytes(r.Budget.Bytes), r.Budget.Source, r.Verdict.Level)
			for i, c := range r.Candidates[:min(3, len(r.Candidates))] {
				ram := ""
				if off := c.Best.Offload; off.Layers > 0 {
					ram = fmt.Sprintf("  %d/%d in RAM", off.Layers, c.Model.Experts.LayerCount())
				}
				fmt.Fprintf(&b, "  %d  %-30s %-7s %-4s %6s %5d tok/s %5s%s\n", i+1, c.Model.ID,
					c.Best.Quant.Name, c.Best.KV, format.Tokens(c.Best.MaxContext),
					int(c.Best.TokensPerSecond+0.5), format.Percent(c.Best.Coverage), ram)
			}
		}
	}
	golden(t, "archetypes.golden", b.String())
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/match -update)", err)
	}
	if got != string(want) {
		t.Errorf("%s differs from the golden file; run go test ./internal/match -update and review the diff", name)
	}
}
