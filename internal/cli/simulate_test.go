package cli_test

import (
	"strings"
	"testing"

	"lo0.ai/lo0/internal/cli"
	"lo0.ai/lo0/internal/machine"
)

func TestHardwareFlagsReachTheService(t *testing.T) {
	tests := []struct {
		name string
		args []string
		gpu  string
	}{
		{"card", []string{"match", "--gpu", "RTX 4090", "--ram", "128"}, "NVIDIA GeForce RTX 4090"},
		{"Mac", []string{"match", "--mac", "M4 Max", "--memory", "64"}, "Apple M4 Max"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(fakeService{res: result()})
			if code := h.app.Run(t.Context(), tt.args); code != cli.ExitOK {
				t.Fatalf("exit %d, stderr: %s", code, h.err)
			}
			if h.env.Hardware == nil {
				t.Fatal("the service was built for this machine")
			}
			p := h.env.Hardware.Apply(machine.Profile{})
			if !p.Simulated || p.GPUs[0].Name != tt.gpu {
				t.Errorf("simulated %+v, want %s", p, tt.gpu)
			}
		})
	}
}

func TestMatchWithoutHardwareFlagsAssessesThisMachine(t *testing.T) {
	h := newHarness(fakeService{res: result()})
	if code := h.app.Run(t.Context(), []string{"match"}); code != cli.ExitOK {
		t.Fatalf("exit %d, stderr: %s", code, h.err)
	}
	if h.env.Hardware != nil {
		t.Error("a plain lo0 match simulated hardware")
	}
}

func TestUnknownHardwareFailsBeforeScanning(t *testing.T) {
	h := newHarness(fakeService{res: result()})
	code := h.app.Run(t.Context(), []string{"match", "--gpu", "RTX 4091"})
	if code != cli.ExitFailure {
		t.Fatalf("exit %d, want %d", code, cli.ExitFailure)
	}
	if !strings.Contains(h.err.String(), "RTX 4090") || h.out.Len() != 0 {
		t.Errorf("stderr = %q, stdout = %q; want the closest card and no report", h.err, h.out)
	}
}

func TestCommandsThatChangeThisMachineRefuseHardware(t *testing.T) {
	for _, args := range [][]string{
		{"try", "--gpu", "RTX 4090"},
		{"migrate", "--dry-run", "--mac=M4 Max"},
		{"benchmark", "-memory", "64"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			h := newHarness(fakeService{res: result()})
			if code := h.app.Run(t.Context(), args); code != cli.ExitUsage {
				t.Fatalf("exit %d, want %d", code, cli.ExitUsage)
			}
			if !strings.Contains(h.err.String(), "works on this machine") || h.env.Getenv != nil {
				t.Errorf("stderr = %q, or the service was built", h.err)
			}
		})
	}
}

func TestHarnessArgumentsAreNotHardwareFlags(t *testing.T) {
	h := newHarness(fakeService{res: result()})
	h.app.Run(t.Context(), []string{"try", "--dry-run", "--", "--gpu", "x"})
	if strings.Contains(h.err.String(), "works on this machine") {
		t.Errorf("arguments after -- were read as lo0's: %s", h.err)
	}
}
