// Package match decides which local models can carry a user's Claude Code
// work on their machine, which parts of their setup carry over to the target
// harness, and whether switching makes sense yet.
package match

import (
	"slices"
	"strings"

	"lo0.ai/lo0/internal/machine"
)

// Bandwidth is the memory bandwidth decoding is bound by.
type Bandwidth struct {
	GBps float64 `json:"gbps"`
	// Device is the chip or card the figure is for.
	Device string `json:"device"`
	// Estimated is true when the device is not in lo0's table and the
	// figure is a conservative guess.
	Estimated bool `json:"estimated,omitzero"`
}

type bandwidthSpec struct {
	match string
	// gpuCores distinguishes binned chips with narrower memory buses. Zero
	// matches any core count.
	gpuCores int
	gbps     float64
}

// appleBandwidth is Apple's published unified memory bandwidth. Longer names
// come first so "M4 Max" is tried before "M4".
var appleBandwidth = []bandwidthSpec{
	// Apple quotes the M5 Ultra at 1.2 TB/s.
	{"M5 Ultra", 0, 1200},
	{"M5 Max", 32, 460},
	{"M5 Max", 0, 614},
	{"M5 Pro", 0, 307},
	{"M5", 0, 153},
	// The M6 reaches 170 GB/s with 24 or 32 GB; the 16 GB model stays at 153.
	{"M6", 0, 170},
	{"M4 Max", 32, 410},
	{"M4 Max", 0, 546},
	{"M4 Pro", 0, 273},
	{"M4", 0, 120},
	{"M3 Ultra", 0, 819},
	{"M3 Max", 30, 300},
	{"M3 Max", 0, 400},
	{"M3 Pro", 0, 150},
	{"M3", 0, 100},
	{"M2 Ultra", 0, 800},
	{"M2 Max", 0, 400},
	{"M2 Pro", 0, 200},
	{"M2", 0, 100},
	{"M1 Ultra", 0, 800},
	{"M1 Max", 0, 400},
	{"M1 Pro", 0, 200},
	{"M1", 0, 68},
}

// discreteBandwidth is manufacturer-published GDDR or HBM bandwidth, longer
// names first.
var discreteBandwidth = []bandwidthSpec{
	{"RTX PRO 6000", 0, 1792},
	{"RTX 6000 Ada", 0, 960},
	{"RTX A6000", 0, 768},
	{"A6000", 0, 768},
	{"H200", 0, 4800},
	// The bare H100 and A100 entries are the SXM parts; nvidia-smi names the
	// PCIe and NVL cards and the 40 GB A100s, whose memory is slower.
	{"H100 PCIe", 0, 2000},
	{"H100 NVL", 0, 3900},
	{"H100", 0, 3350},
	{"A100-SXM4-40GB", 0, 1555},
	{"A100-PCIE-40GB", 0, 1555},
	{"A100 80GB PCIe", 0, 1935},
	{"A100", 0, 2039},
	{"L40S", 0, 864},
	{"5090", 0, 1792},
	{"5080", 0, 960},
	{"5070 Ti", 0, 896},
	{"5070", 0, 672},
	{"5060 Ti", 0, 448},
	{"4090", 0, 1008},
	{"4080 SUPER", 0, 736},
	{"4080", 0, 717},
	{"4090 D", 0, 1008},
	{"4070 Ti SUPER", 0, 672},
	{"4070 Ti", 0, 504},
	{"4070 SUPER", 0, 504},
	{"4070", 0, 504},
	{"4060 Ti", 0, 288},
	{"4060", 0, 272},
	{"3090 Ti", 0, 1008},
	{"3090", 0, 936},
	{"3080 Ti", 0, 912},
	{"3080", 0, 760},
	{"3070 Ti", 0, 608},
	{"3070", 0, 448},
	{"3060 Ti", 0, 448},
	{"3060", 0, 360},
	{"7900 XTX", 0, 960},
	{"7900 XT", 0, 800},
	{"7800 XT", 0, 624},
	{"9070 XT", 0, 640},
}

// Conservative fallbacks for hardware lo0 does not recognise.
const (
	fallbackUnifiedGBps  = 100
	fallbackDiscreteGBps = 300
	fallbackCPUGBps      = 50
)

// MemoryBandwidth looks up the bandwidth of the device that will hold the
// model. With several discrete cards, layers run one card after another, so
// the slowest card sets the pace.
func MemoryBandwidth(p machine.Profile) Bandwidth {
	// Without a GPU that can hold a model, the model sits in system memory,
	// however fast an integrated GPU's own figure might be.
	if len(p.GPUs) == 0 || p.ModelMemory().Source == machine.MemoryCPU {
		return Bandwidth{GBps: fallbackCPUGBps, Device: "CPU", Estimated: true}
	}
	var best *Bandwidth
	for _, g := range p.GPUs {
		table := discreteBandwidth
		if g.Vendor == machine.VendorApple {
			table = appleBandwidth
		}
		bw, ok := lookup(table, g)
		if !ok {
			continue
		}
		if best == nil || bw.GBps < best.GBps {
			best = &bw
		}
	}
	if best != nil {
		return *best
	}
	g := p.GPUs[0]
	if g.Unified {
		return Bandwidth{GBps: fallbackUnifiedGBps, Device: g.Name, Estimated: true}
	}
	return Bandwidth{GBps: fallbackDiscreteGBps, Device: g.Name, Estimated: true}
}

func lookup(table []bandwidthSpec, g machine.GPU) (Bandwidth, bool) {
	for _, s := range table {
		if !isDevice(g.Name, s.match) {
			continue
		}
		if s.gpuCores != 0 && s.gpuCores != g.Cores {
			continue
		}
		return Bandwidth{GBps: s.gbps, Device: g.Name}, true
	}
	return Bandwidth{}, false
}

// variants are the words that follow a chip or card name to make it a
// different device: "M4 Pro" is not an M4, nor "RTX 4090 Laptop GPU" an RTX
// 4090. Laptop parts run at lower clocks and often on narrower buses.
var variants = []string{"PRO", "MAX", "ULTRA", "TI", "SUPER", "D", "LAPTOP", "MOBILE"}

// isDevice reports whether a GPU called name is the device a table calls
// entry: entry appears on word boundaries and is not followed by a variant
// word.
func isDevice(name, entry string) bool {
	name, entry = strings.ToUpper(name), strings.ToUpper(entry)
	for i := 0; i < len(name); {
		j := strings.Index(name[i:], entry)
		if j < 0 {
			return false
		}
		start, end := i+j, i+j+len(entry)
		i = start + 1
		if start > 0 && isWordByte(name[start-1]) || end < len(name) && isWordByte(name[end]) {
			continue
		}
		rest := strings.TrimLeft(name[end:], " -_")
		next := rest[:len(rest)-len(strings.TrimLeftFunc(rest, isWordRune))]
		if !slices.Contains(variants, next) {
			return true
		}
	}
	return false
}

func isWordRune(r rune) bool { return r < 128 && isWordByte(byte(r)) }

func isWordByte(b byte) bool {
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9'
}
