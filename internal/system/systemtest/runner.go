// Package systemtest provides a scripted system.Runner for tests.
package systemtest

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"lo0.ai/lo0/internal/system"
)

// Result is the scripted outcome of one command.
type Result struct {
	Output string
	Err    error
}

// Runner is a system.Runner that answers from a script. Commands that are not
// scripted fail with system.ErrNotFound, as if the program were not installed.
type Runner struct {
	// Commands maps a full command line, such as "ollama --version", to its
	// result.
	Commands map[string]Result
	// Paths maps a program name to the path LookPath returns.
	Paths map[string]string

	mu    sync.Mutex
	calls []string
}

var _ system.Runner = (*Runner)(nil)

// Run implements system.Runner.
func (r *Runner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	line := strings.Join(append([]string{name}, args...), " ")
	r.mu.Lock()
	r.calls = append(r.calls, line)
	r.mu.Unlock()
	res, ok := r.Commands[line]
	if !ok {
		return nil, fmt.Errorf("%s: %w", name, system.ErrNotFound)
	}
	return []byte(res.Output), res.Err
}

// LookPath implements system.Runner.
func (r *Runner) LookPath(name string) (string, error) {
	if path, ok := r.Paths[name]; ok {
		return path, nil
	}
	return "", fmt.Errorf("%s: %w", name, system.ErrNotFound)
}

// Calls returns the command lines that Run received, in order.
func (r *Runner) Calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}
