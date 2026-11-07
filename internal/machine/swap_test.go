package machine_test

import (
	"testing"
	"testing/fstest"

	"lo0.ai/lo0/internal/machine"
	"lo0.ai/lo0/internal/system/systemtest"
)

func TestSwapMeter(t *testing.T) {
	darwin := machine.SwapMeter{GOOS: "darwin", Runner: &systemtest.Runner{Commands: map[string]systemtest.Result{
		"vm_stat": {Output: "Mach Virtual Memory Statistics: (page size of 16384 bytes)\nPages free:     9273.\nSwapouts:     60918213.\n"},
	}}}
	if got, err := darwin.SwappedOut(t.Context()); err != nil || got != 60918213*16384 {
		t.Errorf("darwin = %d, %v", got, err)
	}
	linux := machine.SwapMeter{GOOS: "linux", Root: fstest.MapFS{"proc/vmstat": {Data: []byte("pswpin 12\npswpout 300\n")}}}
	if got, err := linux.SwappedOut(t.Context()); err != nil || got != 300*4096 {
		t.Errorf("linux = %d, %v", got, err)
	}
	if _, err := (machine.SwapMeter{GOOS: "windows"}).SwappedOut(t.Context()); err == nil {
		t.Error("windows read a swap count")
	}
}
