package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/charmbracelet/colorprofile"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/redact"
	"lo0.ai/lo0/internal/runtimes"
	"lo0.ai/lo0/internal/ui/report"
)

// MigrateSchema names the JSON migrate prints.
const MigrateSchema = "lo0.migrate/v1"

type migrateFlags struct {
	commonFlags
	to, runtime, model string
	yes, dryRun, undo  bool
}

func (a *App) migrate(ctx context.Context, args []string) error {
	if err := a.refuseHardware("migrate", args); err != nil {
		return err
	}
	var f migrateFlags
	fs := a.flags("migrate", "[--to claude-code|opencode|codex] [--runtime ollama|lmstudio|llamacpp] [--model ID] [--undo] [--dry-run] [--yes] [--json]")
	f.register(fs)
	fs.StringVar(&f.to, "to", "", "the harness to move to: `claude-code`, opencode or codex; lo0 recommends one by default")
	fs.StringVar(&f.runtime, "runtime", "", "the local runtime to use: `ollama`, lmstudio or llamacpp")
	fs.StringVar(&f.model, "model", "", "a catalogue model `ID` instead of the best fit")
	fs.BoolVar(&f.dryRun, "dry-run", false, "show the plan and change nothing")
	fs.BoolVar(&f.yes, "yes", false, "apply the plan without asking")
	fs.BoolVar(&f.undo, "undo", false, "reverse everything lo0 migrate has changed")
	if err := parse(fs, args); err != nil {
		return err
	}
	svc, err := a.service(f.commonFlags)
	if err != nil {
		return err
	}
	if f.undo {
		if f.to != "" || f.runtime != "" || f.model != "" {
			fmt.Fprintln(a.Err, "--undo reverses every change, so it takes no --to, --runtime or --model")
			return errUsage
		}
		plan, err := svc.PlanUndo()
		if err != nil {
			return err
		}
		if len(plan.Steps) == 0 && !f.json {
			_, err := fmt.Fprintln(a.Out, "lo0 migrate has not changed anything, so there is nothing to undo.")
			return err
		}
		// Undo runs without a scan, so only the home directory is known to
		// hide; the journal holds paths under it.
		var hide func(string) string
		if f.redact {
			hide = func(s string) string { return redact.Paths(s, a.Env.Home, nil) }
		}
		return a.run(ctx, svc, f, plan, a.reportOptions(hide), hide)
	}
	assessment, err := observed(ctx, a, !f.json, svc.Assess)
	if err != nil {
		return err
	}
	choice, err := f.choice(assessment)
	if err != nil {
		fmt.Fprintln(a.Err, err)
		return errUsage
	}
	plan, err := svc.PlanMigration(assessment, choice)
	if err != nil {
		return err
	}
	hide := a.hider(f.commonFlags, assessment.Scan)
	return a.run(ctx, svc, f, plan, a.reportOptions(hide), hide)
}

// run shows a plan, asks unless told not to, and applies it.
func (a *App) run(ctx context.Context, svc Service, f migrateFlags, plan migrate.Plan, opts report.Options, hide func(string) string) error {
	if f.json {
		if !f.yes || f.dryRun {
			return a.writeJSON(MigrateSchema, struct {
				Plan migrate.Plan `json:"plan"`
			}{plan}, hide)
		}
		out, applyErr := svc.Migrate(ctx, plan, nil)
		if err := a.writeJSON(MigrateSchema, struct {
			Plan    migrate.Plan    `json:"plan"`
			Outcome migrate.Outcome `json:"outcome"`
		}{plan, out}, hide); err != nil {
			return err
		}
		return applyErr
	}

	if err := a.writeText(report.RenderPlan(plan, opts)); err != nil {
		return err
	}
	switch {
	case f.dryRun:
		return nil
	case !plan.Pending():
		_, err := fmt.Fprintln(a.Err, "\nEverything is already set up.")
		return err
	case !f.yes && f.undo && !a.ask("Undo these changes?", f.command("migrate", "--to", nil)):
		_, err := fmt.Fprintln(a.Err, "Nothing was changed.")
		return err
	case !f.yes && !f.undo && !a.ask("Apply this plan?", f.command("migrate", "--to", nil)):
		_, err := fmt.Fprintln(a.Err, "Nothing was changed.")
		return err
	}
	out, applyErr := svc.Migrate(ctx, plan, a.stepPrinter(plan, opts))
	if errors.Is(applyErr, context.Canceled) {
		return applyErr
	}
	if err := a.writeText(report.RenderOutcome(plan, out, opts)); err != nil {
		return err
	}
	return applyErr
}

func (f migrateFlags) choice(a app.Assessment) (app.MigrationChoice, error) {
	var c app.MigrationChoice
	if f.to != "" {
		t, ok := a.Match.TargetNamed(f.to)
		if !ok {
			return c, fmt.Errorf("unknown harness %q; lo0 can move to %s", f.to, targetIDs(a))
		}
		c.Harness = t.ID
	}
	switch id := runtimes.ID(strings.ToLower(f.runtime)); id {
	case "", runtimes.Ollama, runtimes.LMStudio, runtimes.LlamaCpp:
		c.Runtime = id
	default:
		return c, fmt.Errorf("unknown runtime %q; use ollama, lmstudio or llamacpp", f.runtime)
	}
	c.Model = f.model
	return c, nil
}

func targetIDs(a app.Assessment) string {
	ids := make([]string, 0, len(a.Match.Targets))
	for _, t := range a.Match.Targets {
		ids = append(ids, string(t.ID))
	}
	return strings.Join(ids, " or ")
}

// command is the lo0 command that applies the plan this run showed: the
// flags that chose it, then --yes and any arguments for the harness. Without
// the flags, the command would apply the default plan instead.
func (f migrateFlags) command(name, harnessFlag string, passed []string) string {
	words := []string{"lo0", name}
	if f.undo {
		words = append(words, "--undo")
	}
	for _, fl := range [][2]string{{harnessFlag, f.to}, {"--runtime", f.runtime}, {"--model", f.model}, {"--claude-dir", f.claudeDir}, {"--codex-dir", f.codexDir}} {
		if fl[1] != "" {
			words = append(words, fl[0], migrate.ShellQuote(fl[1]))
		}
	}
	if f.offline {
		words = append(words, "--offline")
	}
	words = append(words, "--yes")
	if len(passed) > 0 {
		words = append(words, "--")
		for _, p := range passed {
			words = append(words, migrate.ShellQuote(p))
		}
	}
	return strings.Join(words, " ")
}

// ask asks on stderr and reads one line from stdin. Without a terminal
// there is nobody to ask, so the answer is no and the user is told the
// command that applies the plan.
func (a *App) ask(question, command string) bool {
	if !a.Term.IsTerminal(a.In) {
		fmt.Fprintln(a.Err, "\nRun "+command+" to apply this plan.")
		return false
	}
	fmt.Fprint(a.Err, "\n"+question+" [y/N] ")
	line, err := bufio.NewReader(a.In).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

// stepPrinter writes one line per step to stderr as it finishes. On a
// terminal, a running step's progress is redrawn in place.
func (a *App) stepPrinter(p migrate.Plan, opts report.Options) migrate.Observer {
	live := a.Term.IsTerminal(a.Err)
	// Like stdout, stderr loses its colour when it is not a terminal or
	// NO_COLOR is set.
	w := colorprofile.NewWriter(a.Err, a.Env.Environ)
	var mu sync.Mutex
	return func(e migrate.Event) {
		mu.Lock()
		defer mu.Unlock()
		s := p.Steps[e.Step]
		line := "  " + report.StepMark(e.State, opts) + " " + s.Title
		switch e.State {
		case migrate.StateRunning:
			if live {
				fmt.Fprint(w, "\r\x1b[K"+line)
			}
		case migrate.StateProgress:
			if live {
				fmt.Fprint(w, "\r\x1b[K"+line+opts.Theme.Muted.Render("  "+report.Progress(s, e.Done, e.Total)))
			}
		default:
			if live {
				fmt.Fprint(w, "\r\x1b[K")
			}
			fmt.Fprintln(w, line)
		}
	}
}
