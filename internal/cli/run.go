package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/codex"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/redact"
	"lo0.ai/lo0/internal/scan"
	"lo0.ai/lo0/internal/ui/progress"
	"lo0.ai/lo0/internal/ui/report"
	"lo0.ai/lo0/internal/ui/theme"
	"lo0.ai/lo0/internal/wiring"
)

// commonFlags are the flags every scanning command accepts.
type commonFlags struct {
	json      bool
	claudeDir string
	codexDir  string
	offline   bool
	redact    bool
	// hardware replaces this machine for match, ui and web.
	hardware match.HardwareQuery
}

func (c *commonFlags) register(fs *flag.FlagSet) {
	fs.BoolVar(&c.json, "json", false, "print the full result as JSON and nothing else")
	fs.StringVar(&c.claudeDir, "claude-dir", "", "read Claude Code files from `DIR` instead of ~/.claude")
	fs.StringVar(&c.codexDir, "codex-dir", "", "read Codex files from `DIR` instead of ~/.codex")
	fs.BoolVar(&c.offline, "offline", false, "do not look up the newest Claude Code release")
	fs.BoolVar(&c.redact, "redact", false, "replace your project paths and home directory, to share the output")
}

// registerHardware adds the flags that name hardware to assess.
func (c *commonFlags) registerHardware(fs *flag.FlagSet) {
	fs.StringVar(&c.hardware.GPU, "gpu", "", "assess a PC with the graphics card `NAME`, such as \"RTX 4090\", instead of this machine")
	fs.IntVar(&c.hardware.VRAM, "vram", 0, "video memory per card in `GB`, for cards sold in several sizes")
	fs.IntVar(&c.hardware.GPUs, "gpus", 0, "number of cards, `N` (default 1)")
	fs.IntVar(&c.hardware.RAM, "ram", 0, "system memory beside the cards in `GB` (default this machine's)")
	fs.StringVar(&c.hardware.Mac, "mac", "", "assess a Mac with the chip `NAME`, such as \"M4 Max\", instead of this machine")
	fs.IntVar(&c.hardware.Memory, "memory", 0, "the Mac's unified memory in `GB`")
}

func (a *App) service(c commonFlags) (Service, error) {
	env := a.Env
	if !c.hardware.IsZero() {
		sim, err := match.Simulate(c.hardware)
		if err != nil {
			return nil, err
		}
		env.Hardware = &sim
	}
	if c.claudeDir != "" {
		env = env.WithOverride("CLAUDE_CONFIG_DIR", c.claudeDir)
	}
	if c.codexDir != "" {
		env = env.WithOverride(codex.HomeVar, c.codexDir)
	}
	if c.offline {
		env = env.WithOverride(wiring.OfflineVar, "1")
	}
	factory := a.NewService
	if factory == nil {
		factory = SystemService
	}
	return factory(env)
}

// observed runs a scanning use case, with live progress on stderr when it is
// a terminal and the output is meant for a person.
func observed[T any](ctx context.Context, a *App, interactive bool, run func(context.Context, scan.Observer) (T, error)) (T, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if !interactive || !a.Term.IsTerminal(a.Err) {
		return run(ctx, nil)
	}

	opts := []tea.ProgramOption{tea.WithOutput(a.Err)}
	if a.Term.IsTerminal(a.In) {
		opts = append(opts, tea.WithInput(a.In))
	} else {
		opts = append(opts, tea.WithInput(nil))
	}
	p := tea.NewProgram(progress.New(theme.New(a.Term.DarkBackground()), cancel), opts...)

	var (
		result T
		runErr error
		done   = make(chan struct{})
	)
	go func() {
		defer close(done)
		result, runErr = run(ctx, func(e scan.Event) { p.Send(progress.EventMsg(e)) })
		p.Send(progress.DoneMsg{})
	}()
	final, err := p.Run()
	if err != nil {
		cancel()
		<-done
		return result, fmt.Errorf("progress display: %w", err)
	}
	<-done
	if m, ok := final.(progress.Model); ok && m.Interrupted() {
		return result, context.Canceled
	}
	return result, runErr
}

// writeJSON prints v with its schema name and the lo0 version, passing it
// through hide first.
func (a *App) writeJSON(schema string, v any, hide func(string) string) error {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetIndent("", "  ")
	if err := enc.Encode(struct {
		Schema  string `json:"schema"`
		Version string `json:"lo0Version"`
		Result  any    `json:"result"`
	}{schema, a.Version, v}); err != nil {
		return err
	}
	_, err := io.WriteString(a.Out, apply(hide, b.String()))
	return err
}

// hider returns what --redact asks for: a function that removes the
// scan's project paths and home directory, or nil when nothing is hidden.
func (a *App) hider(c commonFlags, res scan.Result) func(string) string {
	if !c.redact {
		return nil
	}
	projects := app.ProjectPaths(res)
	return func(s string) string { return redact.Paths(s, a.Env.Home, projects) }
}

// apply runs hide over s, if there is one.
func apply(hide func(string) string, s string) string {
	if hide == nil {
		return s
	}
	return hide(s)
}

// reportOptions returns rendering options for stdout. hide, if not nil,
// rewrites paths for sharing.
func (a *App) reportOptions(hide func(string) string) report.Options {
	return report.Options{
		Width: a.Term.Width(a.Out),
		Theme: theme.New(a.Term.IsTerminal(a.Out) && a.Term.DarkBackground()),
		Home:  a.Env.Home,
		Hide:  hide,
	}
}

// writeText prints styled text, downsampling or stripping colour to suit
// stdout and NO_COLOR.
func (a *App) writeText(s string) error {
	_, err := io.WriteString(colorprofile.NewWriter(a.Out, a.Env.Environ), s)
	return err
}

var hardwareFlags = []string{"gpu", "vram", "gpus", "ram", "mac", "memory"}

// refuseHardware says why a command that changes or measures this machine
// takes no simulated hardware, in place of an unknown-flag error.
func (a *App) refuseHardware(command string, args []string) error {
	for _, arg := range args {
		if arg == "--" {
			return nil
		}
		name, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if strings.HasPrefix(arg, "-") && slices.Contains(hardwareFlags, name) {
			fmt.Fprintf(a.Err, "lo0 %s works on this machine, so it takes no --%s. Simulated hardware is for lo0 match, ui and web.\n", command, name)
			return errUsage
		}
	}
	return nil
}
