package report_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/machine"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/ui/report"
	"lo0.ai/lo0/internal/ui/theme"
)

func simulated(t *testing.T, q match.HardwareQuery) app.Assessment {
	t.Helper()
	sim, err := match.Simulate(q)
	if err != nil {
		t.Fatal(err)
	}
	a := sampleAssessment()
	a.Scan.Machine = sim.Apply(a.Scan.Machine)
	a.Match = match.Match(a.Scan, sampleCatalog(), harnesses())
	return a
}

func TestRenderMatchSimulatedGolden(t *testing.T) {
	a := simulated(t, match.HardwareQuery{GPU: "RTX 4090", GPUs: 2, RAM: 128})
	got := filepath.ToSlash(report.RenderMatch(a, report.Options{Width: 80, Theme: theme.Plain(), Home: home()}))
	golden := filepath.Join("testdata", "match_simulated_80.golden")
	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("reading golden file (run go test -update): %v", err)
	}
	if got != string(want) {
		t.Errorf("simulated match report differs from %s; run go test ./internal/ui/report -update and review the diff\n\ngot:\n%s", golden, got)
	}
}

func TestSimulatedMachine(t *testing.T) {
	tests := []struct {
		name  string
		query match.HardwareQuery
		want  string
	}{
		{"one card", match.HardwareQuery{GPU: "RTX 4090", RAM: 64}, "Simulated machine: NVIDIA GeForce RTX 4090 with 24 GiB of VRAM, beside 64 GiB of system memory."},
		{"two cards", match.HardwareQuery{GPU: "RTX 3090", GPUs: 2, RAM: 128}, "Simulated machine: 2 × NVIDIA GeForce RTX 3090 with 24 GiB of VRAM each, beside 128 GiB of system memory."},
		{"Mac", match.HardwareQuery{Mac: "M4 Max", Memory: 128}, "Simulated machine: Apple M4 Max with a 40-core GPU and 128 GiB of unified memory."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := simulated(t, tt.query)
			got := report.SimulatedMachine(a.Scan.Machine)
			if !strings.HasPrefix(got, tt.want) || !strings.Contains(got, "sessions and setup are real") {
				t.Errorf("got %q, want it to start %q", got, tt.want)
			}
			if chips := report.MachineChips(a.Scan.Machine, a.Match.Budget); chips[0] != "simulated" {
				t.Errorf("chips = %q, want simulated first", chips)
			}
		})
	}
	if got := report.SimulatedMachine(machine.Profile{GPUs: []machine.GPU{{Name: "NVIDIA GeForce RTX 4090"}}}); got != "" {
		t.Errorf("a real machine reads %q", got)
	}
}
