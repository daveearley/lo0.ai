package web

import (
	"testing"

	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/machine"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/scan"
	"lo0.ai/lo0/internal/usage"
)

func TestModelPageShowsExpertsInSystemMemory(t *testing.T) {
	cat, err := catalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	m := match.Match(scan.Result{
		Machine: machine.Profile{
			Memory: machine.Memory{TotalBytes: 32 << 30},
			GPUs:   []machine.GPU{{Name: "NVIDIA GeForce RTX 4060", Vendor: machine.VendorNVIDIA, VRAMBytes: 8 << 30}},
		},
		Usage: usage.Profile{Sessions: 3, Context: usage.Context{P50: 40_000, Peaks: []int{20_000, 40_000, 90_000}}},
	}, cat, nil)
	best, ok := m.Best()
	if !ok || !best.Best.Offloads() {
		t.Fatalf("best = %+v, want experts in system memory on an 8 GB card", best.Best)
	}
	v := candidateView(best, m)
	if len(v.Offload) != 3 || !v.OptionsOffload {
		t.Errorf("view Offload = %q, OptionsOffload = %v", v.Offload, v.OptionsOffload)
	}
	marked := 0
	for _, o := range v.Options {
		if o.Best {
			marked++
			if o.InRAM == "" {
				t.Errorf("best row %+v does not say how many layers' experts are in system memory", o)
			}
		}
	}
	if marked != 1 {
		t.Errorf("%d rows marked best, want 1", marked)
	}
}
