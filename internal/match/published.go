package match

import (
	"lo0.ai/lo0/internal/machine"
)

// PublishedAsOf is when the published results below were copied from
// llama.cpp's results discussions.
const PublishedAsOf = "5 Nov 2026"

// Where the published results come from.
const (
	AppleResultsURL = "https://github.com/ggml-org/llama.cpp/discussions/4167"
	CUDAResultsURL  = "https://github.com/ggml-org/llama.cpp/discussions/15013"
)

// The model every published result was measured with: Llama 2 7B at Q4_0,
// 3.56 GiB of weights and 6.74 billion parameters, reading a 512-token
// prompt (pp512) and writing 128 tokens (tg128).
const (
	referenceWeightBytes = 3.56 * (1 << 30)
	referenceParams      = 6.74e9
)

// published is one device's result for the reference model, in tokens a
// second.
type published struct {
	match    string
	gpuCores int
	read     float64
	write    float64
}

// applePublished is the Q4_0 column of llama.cpp's Apple Silicon table.
// The M1 to M4 rows were measured with a 2023 build and the M5 and M6 rows
// with 2026 builds, which read faster on the same chip, so older chips may be
// slightly understated. Longer names come first so "M4 Max" is tried
// before "M4".
var applePublished = []published{
	{"M6", 8, 988.93, 35.74},
	{"M5 Ultra", 80, 4944.66, 179.10},
	{"M5 Max", 40, 3219.99, 119.92},
	{"M5 Pro", 16, 1340.44, 67.59},
	{"M5 Pro", 20, 1620.64, 66.33},
	{"M5", 10, 722.79, 31.88},
	{"M4 Max", 32, 713.93, 69.95},
	{"M4 Max", 40, 885.68, 83.06},
	{"M4 Pro", 16, 364.06, 49.64},
	{"M4 Pro", 20, 439.78, 50.74},
	{"M4", 10, 221.29, 24.11},
	{"M3 Ultra", 60, 1073.09, 88.40},
	{"M3 Ultra", 80, 1471.24, 92.14},
	{"M3 Max", 30, 567.59, 56.58},
	{"M3 Max", 40, 759.7, 66.31},
	{"M3 Pro", 14, 269.49, 30.65},
	{"M3 Pro", 18, 341.67, 30.74},
	{"M3", 10, 186.75, 21.34},
	{"M2 Ultra", 60, 1013.81, 88.64},
	{"M2 Ultra", 76, 1238.48, 94.27},
	{"M2 Max", 30, 537.6, 60.99},
	{"M2 Max", 38, 671.31, 65.95},
	{"M2 Pro", 16, 294.24, 37.87},
	{"M2 Pro", 19, 341.19, 38.86},
	{"M2", 8, 145.91, 21.7},
	{"M2", 10, 179.57, 21.91},
	{"M1 Ultra", 48, 772.24, 74.93},
	{"M1 Ultra", 64, 1030.04, 83.73},
	{"M1 Max", 24, 400.26, 54.61},
	{"M1 Max", 32, 530.06, 61.19},
	{"M1 Pro", 14, 232.55, 35.52},
	{"M1 Pro", 16, 266.25, 36.41},
	{"M1", 7, 107.81, 14.19},
	{"M1", 8, 117.96, 14.15},
}

// cudaPublished is the scoreboard without flash attention from llama.cpp's
// CUDA results, longer names first.
var cudaPublished = []published{
	{"RTX 5090 Laptop", 0, 6667.06, 156.49},
	{"RTX 4060 Ti", 0, 3394.63, 63.86},
	{"RTX A6000", 0, 4913.93, 138.73},
	{"RTX 5090", 0, 14073.41, 290.02},
	{"RTX 5080", 0, 8297.36, 181.99},
	{"RTX 4090", 0, 11992.70, 186.21},
	{"RTX 4080", 0, 8031.64, 142.49},
	{"RTX 3090", 0, 5174.69, 158.16},
	{"RTX 3080", 0, 5013.86, 139.65},
	{"RTX 3060", 0, 2137.50, 75.57},
	{"A100", 0, 4849.53, 190.88},
	{"H100", 0, 9918.34, 267.81},
}

// publishedFor finds the published result for the machine's GPU. Apple
// chips need the same core count, since binned chips differ; a chip with an
// unlisted count takes the nearest listed one.
func publishedFor(p machine.Profile) (published, string, bool) {
	if len(p.GPUs) == 0 || p.ModelMemory().Source == machine.MemoryCPU {
		return published{}, "", false
	}
	g := p.GPUs[0]
	if g.Vendor == machine.VendorApple {
		var best published
		found := false
		for _, r := range applePublished {
			if !isDevice(g.Name, r.match) {
				continue
			}
			if !found || abs(r.gpuCores-g.Cores) < abs(best.gpuCores-g.Cores) {
				best, found = r, true
			}
		}
		return best, AppleResultsURL, found
	}
	for _, r := range cudaPublished {
		if isDevice(g.Name, r.match) {
			return r, CUDAResultsURL, true
		}
	}
	return published{}, "", false
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
