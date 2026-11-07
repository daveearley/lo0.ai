// Package system wraps process execution so that callers can be tested
// without spawning real programs.
package system

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

// ErrNotFound reports that a program is not installed or not on PATH.
var ErrNotFound = errors.New("program not found")

// Runner runs external programs and locates them on PATH.
type Runner interface {
	// Run executes name with args and returns its combined stdout and stderr.
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
	// LookPath returns the absolute path of name, or an error wrapping
	// ErrNotFound.
	LookPath(name string) (string, error)
}

// ExecRunner is a Runner backed by os/exec.
type ExecRunner struct {
	// Timeout bounds each Run call. Zero means no bound beyond ctx.
	Timeout time.Duration
}

// Run implements Runner.
func (r ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if r.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.Timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	// A grandchild, such as node under Windows' claude.cmd shim, can hold
	// the output pipe open after ctx kills the child; stop waiting for it.
	cmd.WaitDelay = 5 * time.Second
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("%s: %w", name, ErrNotFound)
		}
		return out.Bytes(), fmt.Errorf("running %s: %w", name, err)
	}
	return out.Bytes(), nil
}

// LookPath implements Runner.
func (ExecRunner) LookPath(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, ErrNotFound)
	}
	return path, nil
}
