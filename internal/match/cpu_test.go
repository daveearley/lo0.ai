package match_test

import (
	"slices"
	"strings"
	"testing"

	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/machine"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/usage"
)

// cpuResult is appleResult on a PC whose only GPU, if any, cannot hold a
// model.
func cpuResult(gpus []machine.GPU, peaks ...int) match.Result {
	res := appleResult(peaks...)
	res.Machine = machine.Profile{
		CPU:    machine.CPU{Model: "AMD Ryzen 7 7840U", PhysicalCores: 8},
		Memory: machine.Memory{TotalBytes: uint64(64 * gib)},
		GPUs:   gpus,
	}
	return match.Match(res, testCatalog(), harnesses())
}

func TestMatchWithoutAGPU(t *testing.T) {
	for name, gpus := range map[string][]machine.GPU{
		"no GPU at all":       nil,
		"integrated graphics": {{Name: "AMD Radeon 780M", Vendor: machine.VendorAMD, VRAMBytes: 512 << 20}},
	} {
		t.Run(name, func(t *testing.T) {
			r := cpuResult(gpus, 20_000, 30_000, 40_000)
			if r.Budget.Source != machine.MemoryCPU || r.Budget.Bytes != uint64(48*gib) {
				t.Errorf("Budget = %+v, want 48 GiB on the CPU", r.Budget)
			}
			if r.Bandwidth.Device != "CPU" || !r.Bandwidth.Estimated {
				t.Errorf("Bandwidth = %+v, want the CPU's, estimated", r.Bandwidth)
			}
			v := r.Verdict
			if v.Level == match.LevelReady {
				t.Errorf("a machine without a GPU was called ready: %+v", v)
			}
			if !strings.Contains(v.Headline, "no GPU") {
				t.Errorf("Headline = %q, want it to say there is no GPU", v.Headline)
			}
			if len(v.Reasons) == 0 || !strings.Contains(v.Reasons[0], "no GPU") || !strings.Contains(v.Reasons[0], "on the CPU") {
				t.Errorf("first reason = %q, want it to say there is no GPU and models run on the CPU", v.Reasons)
			}
			if gpus != nil && !strings.Contains(v.Reasons[0], "AMD Radeon 780M") {
				t.Errorf("first reason = %q, want it to name the integrated GPU", v.Reasons[0])
			}
			for _, want := range []string{"reads long contexts far more slowly than a GPU", "lo0 benchmark"} {
				if !slices.ContainsFunc(v.Reasons, func(s string) bool { return strings.Contains(s, want) }) {
					t.Errorf("reasons lack %q:\n%s", want, strings.Join(v.Reasons, "\n"))
				}
			}
		})
	}
}

// Without history there is no session to time, but the warnings about the
// CPU still stand.
func TestMatchWithoutAGPUOrHistory(t *testing.T) {
	res := appleResult(1)
	res.Machine = machine.Profile{Memory: machine.Memory{TotalBytes: uint64(64 * gib)}}
	res.Usage = usage.Profile{}
	v := match.Match(res, testCatalog(), harnesses()).Verdict
	if v.Level != match.LevelNoHistory || !strings.Contains(strings.Join(v.Reasons, "\n"), "no GPU") {
		t.Errorf("Verdict = %s, reasons %q; want no history and the no-GPU warning", v.Level, v.Reasons)
	}
}

// A model too slow to use cannot move work today, so its score is cut
// however well it fits.
func TestTooSlowScoresLow(t *testing.T) {
	cat := &catalog.Catalog{Generated: "2026-11-04", Models: []catalog.Model{model("dense", catalog.TierA, 30, 1024)}}
	cat.Models[0].ActiveParams = cat.Models[0].Params
	res := appleResult(10_000)
	res.Machine = machine.Profile{Memory: machine.Memory{TotalBytes: uint64(128 * gib)}}
	v := match.Match(res, cat, harnesses()).Verdict
	if v.Level != match.LevelNotYet || v.Score >= 50 {
		t.Errorf("Verdict = %s scored %d, want not yet with a low score", v.Level, v.Score)
	}
}

func TestFastEnoughModelOutranksSlowOne(t *testing.T) {
	slow := model("dense-a", catalog.TierA, 16, 1024)
	slow.ActiveParams = slow.Params
	fast := model("small-c", catalog.TierC, 1, 256)
	fast.Params, fast.ActiveParams = 2e9, 2e9
	tests := []struct {
		name      string
		fastLimit int
		want      string
	}{
		{"fast model covers as many sessions", 256 << 10, "small-c"},
		{"slow model covers more sessions", 40 << 10, "dense-a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fast.ContextLength = tt.fastLimit
			res := appleResult(20_000, 30_000, 100_000, 150_000)
			res.Machine = machine.Profile{Memory: machine.Memory{TotalBytes: uint64(64 * gib)}}
			r := match.Match(res, &catalog.Catalog{Models: []catalog.Model{slow, fast}}, harnesses())
			if len(r.Candidates) != 2 {
				t.Fatalf("candidates = %+v, want both models", r.Candidates)
			}
			byID := map[string]match.Candidate{}
			for _, c := range r.Candidates {
				byID[c.Model.ID] = c
			}
			if s, f := byID["dense-a"], byID["small-c"]; s.Best.TokensPerSecond >= 10 || f.Best.TokensPerSecond < 10 || s.Score <= f.Score {
				t.Fatalf("dense-a %.1f tok/s scores %.3f, small-c %.1f tok/s scores %.3f; the test needs a slow model that scores higher",
					s.Best.TokensPerSecond, s.Score, f.Best.TokensPerSecond, f.Score)
			}
			if got := r.Candidates[0].Model.ID; got != tt.want {
				t.Errorf("top pick = %s, want %s", got, tt.want)
			}
		})
	}
}
