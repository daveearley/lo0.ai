package cli

import (
	"os"
	"sync"
	"time"

	"charm.land/lipgloss/v2"
	"golang.org/x/term"

	"lo0.ai/lo0/internal/buildinfo"
	"lo0.ai/lo0/internal/wiring"
)

// FromOS returns an App wired to the real process and terminal.
func FromOS() (*App, error) {
	env, err := wiring.FromOS()
	if err != nil {
		return nil, err
	}
	return &App{
		In:      os.Stdin,
		Out:     os.Stdout,
		Err:     os.Stderr,
		Env:     env,
		Term:    NewOSTerminal(os.Stdin, os.Stdout),
		Version: buildinfo.Version(),
		Save:    CreateFile,
		Now:     time.Now,
		Browser: SystemBrowser(env.GOOS),
	}, nil
}

// OSTerminal is a Terminal backed by real file descriptors.
type OSTerminal struct {
	dark func() bool
}

// NewOSTerminal returns a Terminal for the given input and output. The
// background colour is queried at most once, because each query is a round
// trip to the terminal.
func NewOSTerminal(in, out *os.File) *OSTerminal {
	return &OSTerminal{dark: sync.OnceValue(func() bool { return lipgloss.HasDarkBackground(in, out) })}
}

type fder interface{ Fd() uintptr }

// IsTerminal implements Terminal.
func (*OSTerminal) IsTerminal(stream any) bool {
	f, ok := stream.(fder)
	return ok && term.IsTerminal(int(f.Fd()))
}

// Width implements Terminal.
func (*OSTerminal) Width(stream any) int {
	f, ok := stream.(fder)
	if !ok {
		return 0
	}
	w, _, err := term.GetSize(int(f.Fd()))
	if err != nil {
		return 0
	}
	return w
}

// DarkBackground implements Terminal. It asks the terminal for its
// background colour and assumes dark if the terminal does not answer.
func (t *OSTerminal) DarkBackground() bool {
	return t.dark()
}
