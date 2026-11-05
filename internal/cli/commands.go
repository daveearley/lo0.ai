package cli

import (
	"context"
	"errors"
	"io"
	"os/exec"

	tea "charm.land/bubbletea/v2"

	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/ui/report"
	"lo0.ai/lo0/internal/ui/tui"
)

// JSON schema names. A schema changes only when a field is removed or
// changes meaning.
const (
	ScanSchema  = "lo0.scan/v1"
	MatchSchema = "lo0.match/v1"
)

func (a *App) scan(ctx context.Context, args []string) error {
	var c commonFlags
	fs := a.flags("scan", "[--json] [--redact] [--claude-dir DIR] [--offline]")
	c.register(fs)
	if err := parse(fs, args); err != nil {
		return err
	}
	svc, err := a.service(c)
	if err != nil {
		return err
	}
	res, err := observed(ctx, a, !c.json, svc.Scan)
	if err != nil {
		return err
	}
	hide := a.hider(c, res)
	if c.json {
		return a.writeJSON(ScanSchema, res, hide)
	}
	return a.writeText(apply(hide, report.Render(res, a.reportOptions(hide))))
}

func (a *App) match(ctx context.Context, args []string) error {
	var c commonFlags
	fs := a.flags("match", "[--json] [--redact] [--claude-dir DIR] [--offline]")
	c.register(fs)
	if err := parse(fs, args); err != nil {
		return err
	}
	svc, err := a.service(c)
	if err != nil {
		return err
	}
	assessment, err := observed(ctx, a, !c.json, svc.Assess)
	if err != nil {
		return err
	}
	hide := a.hider(c, assessment.Scan)
	if c.json {
		return a.writeJSON(MatchSchema, assessment, hide)
	}
	return a.writeText(apply(hide, report.RenderMatch(assessment, a.reportOptions(hide))))
}

func (a *App) ui(ctx context.Context, args []string) error {
	var c commonFlags
	fs := a.flags("ui", "[--claude-dir DIR] [--offline]")
	fs.StringVar(&c.claudeDir, "claude-dir", "", "read Claude Code files from `DIR` instead of ~/.claude")
	fs.BoolVar(&c.offline, "offline", false, "do not look up the newest Claude Code release")
	if err := parse(fs, args); err != nil {
		return err
	}
	if !a.Term.IsTerminal(a.Out) {
		return errors.New("the interactive interface needs a terminal; run lo0 match for a printable report")
	}
	svc, err := a.service(c)
	if err != nil {
		return err
	}
	// In the interface Ctrl-C is a key, not a signal. The only interrupt that
	// reaches lo0 is one pressed in a harness lo0 started, which belongs to
	// that harness, so the interface must not stop for it.
	ctx = context.WithoutCancel(ctx)
	return tui.Run(tui.Options{
		Service:        svc,
		DarkBackground: true, // the interface asks the terminal itself and restyles
		Home:           a.Env.Home,
		Version:        a.Version,
		Save:           a.Save,
		Now:            a.Now,
		Actions:        svc,
		Session: func(s migrate.Session) tea.ExecCommand {
			return attached{exec.CommandContext(ctx, s.Program, s.Args...)}.withEnv(s.Environ(a.Env.Environ))
		},
	}, tea.WithContext(ctx), tea.WithInput(a.In), tea.WithOutput(a.Out))
}

// attached runs a harness in the terminal the interface hands over.
type attached struct{ *exec.Cmd }

func (c attached) withEnv(env []string) attached {
	c.Env = env
	return c
}

func (c attached) SetStdin(r io.Reader)  { c.Stdin = r }
func (c attached) SetStdout(w io.Writer) { c.Stdout = w }
func (c attached) SetStderr(w io.Writer) { c.Stderr = w }
