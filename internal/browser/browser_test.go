package browser_test

import (
	"strconv"
	"strings"
	"testing"

	"lo0.ai/lo0/internal/browser"
	"lo0.ai/lo0/internal/match"
)

func TestAssessDescribesTheNamedHardware(t *testing.T) {
	tests := []struct {
		name    string
		req     browser.Request
		command string
	}{
		{"card", browser.Request{GPU: "RTX 4090", RAM: 64}, `lo0 match --gpu "RTX 4090" --ram 64`},
		{"two cards", browser.Request{GPU: "RTX 3090", GPUs: 2, RAM: 128}, `lo0 match --gpu "RTX 3090" --gpus 2 --ram 128`},
		{"card sold in sizes", browser.Request{GPU: "RTX 4060 Ti", VRAM: 16, RAM: 32}, `lo0 match --gpu "RTX 4060 Ti" --vram 16 --ram 32`},
		{"Mac", browser.Request{Mac: "M4 Max", Memory: 64}, `lo0 match --mac "M4 Max" --memory 64`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := browser.Assess(t.Context(), tt.req)
			if err != nil {
				t.Fatal(err)
			}
			if a.Command != tt.command {
				t.Errorf("command = %s, want %s", a.Command, tt.command)
			}
			if !strings.Contains(a.Models, "tok/s") || strings.Contains(a.Models, "\x1b") {
				t.Errorf("models = %q, want a plain table", a.Models)
			}
			if len(a.Reasons) == 0 || !strings.HasPrefix(a.Reasons[0], "Best fit for this machine") {
				t.Errorf("reasons = %q, want the best fit", a.Reasons)
			}
		})
	}
}

func TestAssessKeepsModelNamesWhole(t *testing.T) {
	for _, width := range []int{0, 40, 56, 79} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			a, err := browser.Assess(t.Context(), browser.Request{GPU: "RTX 4090", RAM: 64, Width: width})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(a.Models, "Devstral-Small-2-24B ") {
				t.Errorf("models = %q, want whole model names", a.Models)
			}
		})
	}
}

func TestAssessRefusesUnknownHardware(t *testing.T) {
	_, err := browser.Assess(t.Context(), browser.Request{GPU: "RTX 4091"})
	if err == nil || !strings.Contains(err.Error(), "RTX 4090") {
		t.Errorf("err = %v, want the closest card named", err)
	}
}

func TestKnownListsSimulateableHardware(t *testing.T) {
	k := browser.Known()
	if len(k.GPUs) == 0 || len(k.Macs) == 0 {
		t.Fatalf("known = %+v", k)
	}
	if _, err := match.Simulate(match.HardwareQuery{GPU: k.GPUs[0].Name, VRAM: k.GPUs[0].VRAM[0]}); err != nil {
		t.Error(err)
	}
}
