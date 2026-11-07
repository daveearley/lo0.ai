package wiring_test

import (
	"testing"

	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/wiring"
)

func TestHardwareOnlyAssessesTheNamedHardware(t *testing.T) {
	sim, err := match.Simulate(match.HardwareQuery{Mac: "M4 Max", Memory: 64})
	if err != nil {
		t.Fatal(err)
	}
	svc, err := wiring.HardwareOnly(sim)
	if err != nil {
		t.Fatal(err)
	}
	a, err := svc.Assess(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Scan.Machine.Simulated || a.Scan.Usage.Sessions != 0 || len(a.Scan.Runtimes) != 0 {
		t.Errorf("scan = %+v, want only the simulated machine", a.Scan)
	}
	if len(a.Match.Candidates) == 0 {
		t.Error("no models fit a 64 GB M4 Max")
	}
	if a.Match.Verdict.Level != match.LevelNoHistory {
		t.Errorf("verdict = %v, want no history", a.Match.Verdict.Level)
	}
}
