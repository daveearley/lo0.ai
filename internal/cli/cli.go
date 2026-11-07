// Package cli is lo0's command-line front end. It parses arguments, picks
// the output mode and calls the application service; it holds no logic of
// its own about machines, models or setups.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/runtimes"
	"lo0.ai/lo0/internal/scan"
	"lo0.ai/lo0/internal/wiring"
)

// Exit codes.
const (
	ExitOK          = 0
	ExitFailure     = 1
	ExitUsage       = 2
	ExitInterrupted = 130
)

// Terminal answers questions about the streams lo0 writes to.
type Terminal interface {
	// IsTerminal reports whether stream is an interactive terminal.
	IsTerminal(stream any) bool
	// Width returns the column count of stream, or 0 if unknown.
	Width(stream any) int
	// DarkBackground reports whether the terminal background is dark.
	DarkBackground() bool
}

// Service is the part of the application service the command line uses.
type Service interface {
	Scan(ctx context.Context, observe scan.Observer) (scan.Result, error)
	Assess(ctx context.Context, observe scan.Observer) (app.Assessment, error)
	PlanMigration(a app.Assessment, choice app.MigrationChoice) (migrate.Plan, error)
	PlanTry(a app.Assessment, choice app.MigrationChoice) (migrate.Plan, error)
	SessionReport(ctx context.Context, a app.Assessment, p migrate.Plan, started, ended time.Time, exit int) (app.SessionReport, error)
	PlanUndo() (migrate.Plan, error)
	Inventory(a app.Assessment) (app.Inventory, error)
	Harnesses(ctx context.Context, a app.Assessment) []app.HarnessView
	PlanInstall(a app.Assessment, id harness.ID) (migrate.Plan, error)
	PlanRemove(a app.Assessment, rt runtimes.ID, model string) (migrate.Plan, error)
	PlanPull(a app.Assessment, choice app.MigrationChoice) (migrate.Plan, error)
	PlanBenchmark(a app.Assessment, model string) (migrate.Plan, error)
	Migrate(ctx context.Context, p migrate.Plan, observe migrate.Observer) (migrate.Outcome, error)
}

// ServiceFactory builds the service for an environment.
type ServiceFactory func(wiring.Environment) (Service, error)

// SystemService is the default ServiceFactory: the real machine.
func SystemService(env wiring.Environment) (Service, error) {
	svc, err := wiring.Service(env)
	if err != nil {
		return nil, err
	}
	return svc, nil
}

// App is the lo0 command-line program.
type App struct {
	In      io.Reader
	Out     io.Writer
	Err     io.Writer
	Env     wiring.Environment
	Term    Terminal
	Version string
	// NewService builds the service. It defaults to SystemService.
	NewService ServiceFactory
	// Save writes a new file in the current directory and fails, wrapping
	// fs.ErrExist, if one is already there. Now is the clock. The
	// interactive interface offers to save a report only when both are set.
	Save func(name string, data []byte) error
	Now  func() time.Time
	// Launch starts the harness for lo0 try. It defaults to LaunchAttached.
	Launch Launch
	// Browser opens lo0 web's address. Without it, the address is only
	// printed.
	Browser Browser
}

// CreateFile is the default Save: it writes data to a new file named name in
// the current directory and never replaces an existing one.
func CreateFile(name string, data []byte) error {
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("writing %s: %w", name, err)
	}
	return f.Close()
}

var errUsage = errors.New("usage")

type command struct {
	name    string
	summary string
	run     func(ctx context.Context, args []string) error
}

func (a *App) commands() []command {
	return []command{
		{"scan", "Profile this machine, its local runtimes and your Claude Code and Codex history", a.scan},
		{"match", "Find the local models that fit your work and say whether to switch", a.match},
		{"try", "Start a harness on a local model for one session, changing none of its settings", a.try},
		{"migrate", "Set up the recommended model and harness, after showing every change", a.migrate},
		{"benchmark", "Measure a downloaded model's real speed on this machine", a.benchmark},
		{"ui", "Explore the assessment interactively; the default in a terminal", a.ui},
		{"web", "Open the assessment in your browser, served from this machine only", a.web},
		{"version", "Print the lo0 version", a.version},
	}
}

// Run executes args, which exclude the program name, and returns the exit
// code.
func (a *App) Run(ctx context.Context, args []string) int {
	if len(args) == 0 && a.Term.IsTerminal(a.In) && a.Term.IsTerminal(a.Out) {
		args = []string{"ui"}
	}
	if len(args) == 0 || isHelp(args[0]) {
		a.usage(a.Out)
		if len(args) == 0 {
			return ExitUsage
		}
		return ExitOK
	}
	for _, c := range a.commands() {
		if c.name != args[0] {
			continue
		}
		err := c.run(ctx, args[1:])
		if code, ok := errors.AsType[exitCode](err); ok {
			return int(code)
		}
		switch {
		case err == nil:
			return ExitOK
		case errors.Is(err, flag.ErrHelp):
			return ExitOK
		case errors.Is(err, errUsage):
			return ExitUsage
		case errors.Is(err, context.Canceled) && (c.name == "migrate" || c.name == "try"):
			fmt.Fprintf(a.Err, "Stopped. Steps already finished stay done; run lo0 %s again to continue.\n", c.name)
			return ExitInterrupted
		case errors.Is(err, context.Canceled):
			fmt.Fprintln(a.Err, "Scan stopped. Nothing was changed.")
			return ExitInterrupted
		default:
			fmt.Fprintf(a.Err, "lo0 %s: %v\n", c.name, err)
			return ExitFailure
		}
	}
	fmt.Fprintf(a.Err, "lo0: unknown command %q\n\n", args[0])
	a.usage(a.Err)
	return ExitUsage
}

func isHelp(arg string) bool {
	return arg == "-h" || arg == "--help" || arg == "help"
}

func (a *App) usage(w io.Writer) {
	var b strings.Builder
	b.WriteString("lo0 checks whether your Claude Code and Codex work can run on a local model.\n\n")
	b.WriteString("Usage\n  lo0 <command> [flags]\n\nCommands\n")
	for _, c := range a.commands() {
		fmt.Fprintf(&b, "  %-10s %s\n", c.name, c.summary)
	}
	b.WriteString("\nRun 'lo0 <command> -h' for a command's flags.\n")
	fmt.Fprint(w, b.String())
}

func (a *App) version(_ context.Context, args []string) error {
	if err := parse(a.flags("version", ""), args); err != nil {
		return err
	}
	_, err := fmt.Fprintln(a.Out, "lo0", a.Version)
	return err
}

// flags returns a flag set that prints its usage to stderr and reports
// errors instead of exiting.
func (a *App) flags(name, synopsis string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(a.Err)
	fs.Usage = func() {
		fmt.Fprintf(a.Err, "Usage\n  lo0 %s %s\n", name, synopsis)
		var flags strings.Builder
		fs.SetOutput(&flags)
		fs.PrintDefaults()
		fs.SetOutput(a.Err)
		if flags.Len() > 0 {
			fmt.Fprintf(a.Err, "\nFlags\n%s", flags.String())
		}
	}
	return fs
}

// parse parses args and turns flag errors into errUsage, since the flag
// package has already explained the problem.
func parse(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return err
		}
		return errUsage
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(fs.Output(), "unexpected argument %q\n", fs.Arg(0))
		fs.Usage()
		return errUsage
	}
	return nil
}
