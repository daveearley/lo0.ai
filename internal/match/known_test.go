package match_test

import (
	"testing"

	"lo0.ai/lo0/internal/match"
)

func TestEveryKnownGPUSimulates(t *testing.T) {
	gpus := match.KnownGPUs()
	if len(gpus) == 0 {
		t.Fatal("no known GPUs")
	}
	for _, g := range gpus {
		for _, v := range g.VRAM {
			if _, err := match.Simulate(match.HardwareQuery{GPU: g.Name, VRAM: v, RAM: 32}); err != nil {
				t.Errorf("%s with %d GB: %v", g.Name, v, err)
			}
		}
	}
}

func TestEveryKnownMacSimulates(t *testing.T) {
	macs := match.KnownMacs()
	if len(macs) == 0 || macs[0].Chip != "M5 Ultra" {
		t.Fatalf("macs = %v, want the newest first", macs)
	}
	seen := map[string]bool{}
	for _, m := range macs {
		if seen[m.Chip] {
			t.Errorf("%s listed twice", m.Chip)
		}
		seen[m.Chip] = true
		if _, err := match.Simulate(match.HardwareQuery{Mac: m.Chip, Memory: 32}); err != nil {
			t.Errorf("%s: %v", m.Chip, err)
		}
	}
}
