package wiring

import (
	"context"
	"errors"
	"testing"

	"lo0.ai/lo0/internal/machine"
	"lo0.ai/lo0/internal/match"
)

type proberFunc func(context.Context) (machine.Profile, error)

func (f proberFunc) Probe(ctx context.Context) (machine.Profile, error) { return f(ctx) }

func TestSimulatedProberReplacesTheHardware(t *testing.T) {
	sim, err := match.Simulate(match.HardwareQuery{GPU: "RTX 4090"})
	if err != nil {
		t.Fatal(err)
	}
	partial := errors.New("no GPU tool")
	real := proberFunc(func(context.Context) (machine.Profile, error) {
		return machine.Profile{OS: machine.OS{GOOS: "linux"}, Memory: machine.Memory{TotalBytes: 32 << 30}}, partial
	})
	p, err := simulated{real, sim}.Probe(t.Context())
	if !errors.Is(err, partial) {
		t.Errorf("err = %v, want the real probe's warning passed on", err)
	}
	if !p.Simulated || len(p.GPUs) != 1 || p.GPUs[0].Name != "NVIDIA GeForce RTX 4090" || p.Memory.TotalBytes != 32<<30 {
		t.Errorf("profile = %+v, want the simulated card beside this machine's memory", p)
	}
}
