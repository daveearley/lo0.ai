package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"time"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/ui/report"
)

// TrySchema names the JSON try prints.
const TrySchema = "lo0.try/v1"

// Launch runs program attached to the terminal with env as its whole
// environment and returns its exit code. It returns an error only when the
// program could not be started.
type Launch func(ctx context.Context, program string, args, env []string) (int, error)

// LaunchAttached is the default Launch: the program shares lo0's terminal.
func LaunchAttached(ctx context.Context, program string, args, env []string) (int, error) {
	// The terminal sends Ctrl-C to the harness as well as to lo0, and
	// harnesses use it to clear input or stop a reply, so lo0's cancellation
	// must not kill the session. lo0 just waits for it to exit.
	cmd := exec.CommandContext(context.WithoutCancel(ctx), program, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = env
	err := cmd.Run()
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		return exit.ExitCode(), nil
	}
	if err != nil {
		return 0, fmt.Errorf("starting %s: %w", program, err)
	}
	return 0, nil
}

// exitCode passes a session's exit code on as lo0's own.
type exitCode int

func (e exitCode) Error() string { return fmt.Sprintf("exit status %d", int(e)) }

func (a *App) try(ctx context.Context, args []string) error {
	if err := a.refuseHardware("try", args); err != nil {
		return err
	}
	var f migrateFlags
	fs := a.flags("try", "[--harness claude-code|opencode|codex] [--runtime ollama|lmstudio|llamacpp] [--model ID] [--dry-run] [--yes] [--json] [-- harness arguments]")
	f.register(fs)
	fs.StringVar(&f.to, "harness", "", "the harness to start: `claude-code`, opencode or codex; lo0 recommends one by default")
	fs.StringVar(&f.runtime, "runtime", "", "the local runtime to use: `ollama`, lmstudio or llamacpp")
	fs.StringVar(&f.model, "model", "", "a catalogue model `ID` instead of the best fit")
	fs.BoolVar(&f.dryRun, "dry-run", false, "show the plan and start nothing")
	fs.BoolVar(&f.yes, "yes", false, "run the plan's steps without asking")
	// Everything after -- belongs to the harness.
	own, passed := args, []string(nil)
	if i := slices.Index(args, "--"); i >= 0 {
		own, passed = args[:i], args[i+1:]
	}
	if err := parse(fs, own); err != nil {
		return err
	}
	interactive := !f.dryRun && !f.json
	if interactive && (!a.Term.IsTerminal(a.In) || !a.Term.IsTerminal(a.Out)) {
		return errors.New("a session needs a terminal; run lo0 try --dry-run to see the plan")
	}
	svc, err := a.service(f.commonFlags)
	if err != nil {
		return err
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
	plan, err := svc.PlanTry(assessment, choice)
	if err != nil {
		return err
	}
	plan.Session.Args = append(plan.Session.Args, passed...)
	hide := a.hider(f.commonFlags, assessment.Scan)
	if f.json {
		return a.writeJSON(TrySchema, struct {
			Plan migrate.Plan `json:"plan"`
		}{plan}, hide)
	}
	opts := a.reportOptions(hide)
	if err := a.writeText(report.RenderPlan(plan, opts)); err != nil {
		return err
	}
	if f.dryRun {
		return nil
	}
	if plan.Changes() && !f.yes && !a.ask("Run these steps and start "+plan.Session.Program+"?", f.command("try", "--harness", passed)) {
		_, err := fmt.Fprintln(a.Err, "Nothing was changed.")
		return err
	}
	fmt.Fprintln(a.Err)
	out, applyErr := svc.Migrate(ctx, plan, a.stepPrinter(plan, opts))
	if errors.Is(applyErr, context.Canceled) {
		return applyErr
	}
	if applyErr != nil || !out.Complete() {
		if err := a.writeText(report.RenderOutcome(plan, out, opts)); err != nil {
			return err
		}
		return applyErr
	}
	return a.session(ctx, svc, assessment, plan, opts)
}

// session starts the plan's harness and, once it exits, reports what the
// session did and says how to keep the setup.
func (a *App) session(ctx context.Context, svc Service, assessment app.Assessment, plan migrate.Plan, opts report.Options) error {
	launch := a.Launch
	if launch == nil {
		launch = LaunchAttached
	}
	s := *plan.Session
	fmt.Fprintf(a.Err, "\nStarting %s with %s through %s.\n\n", s.Program, plan.Model, plan.RuntimeName)
	started := a.now()
	code, err := launch(ctx, s.Program, s.Args, s.Environ(a.Env.Environ))
	if err != nil {
		return err
	}
	// A Ctrl-C meant for the session also cancels ctx; the report is still
	// worth reading.
	r, err := svc.SessionReport(context.WithoutCancel(ctx), assessment, plan, started, a.now(), code)
	if err != nil && !r.Found {
		fmt.Fprintln(a.Err, "\nCould not read the session back:", err)
	}
	fmt.Fprintln(a.Err)
	if err := a.writeText(report.RenderSession(r, opts)); err != nil {
		return err
	}
	fmt.Fprintln(a.Err)
	for _, line := range plan.Then {
		fmt.Fprintln(a.Err, line)
	}
	if code != 0 {
		return exitCode(code)
	}
	return nil
}

// now is the App's clock.
func (a *App) now() time.Time {
	if a.Now == nil {
		return time.Now()
	}
	return a.Now()
}
