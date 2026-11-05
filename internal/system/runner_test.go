package system_test

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"lo0.ai/lo0/internal/system"
)

func TestExecRunnerMissingProgram(t *testing.T) {
	r := system.ExecRunner{}
	if _, err := r.LookPath("lo0-definitely-not-installed"); !errors.Is(err, system.ErrNotFound) {
		t.Fatalf("LookPath error = %v, want ErrNotFound", err)
	}
	if _, err := r.Run(t.Context(), "lo0-definitely-not-installed"); !errors.Is(err, system.ErrNotFound) {
		t.Fatalf("Run error = %v, want ErrNotFound", err)
	}
}

func TestExecRunnerCapturesOutput(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}
	r := system.ExecRunner{Timeout: time.Minute}
	out, err := r.Run(t.Context(), "go", "env", "GOOS")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.TrimSpace(string(out)) == "" {
		t.Fatal("Run returned no output")
	}
}

func TestExecRunnerReportsFailure(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}
	_, err := system.ExecRunner{}.Run(t.Context(), "go", "lo0-no-such-subcommand")
	if err == nil {
		t.Fatal("Run succeeded, want error")
	}
	if errors.Is(err, system.ErrNotFound) {
		t.Fatalf("Run error = %v, must not be ErrNotFound for an installed program", err)
	}
}
