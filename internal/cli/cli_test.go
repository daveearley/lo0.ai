package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/cli"
	"lo0.ai/lo0/internal/codex"
	harnesspkg "lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/machine"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/runtimes"
	"lo0.ai/lo0/internal/scan"
	"lo0.ai/lo0/internal/usage"
	"lo0.ai/lo0/internal/wiring"
)

type fakeTerminal struct{ tty bool }

func (f fakeTerminal) IsTerminal(any) bool { return f.tty }
func (fakeTerminal) Width(any) int         { return 80 }
func (fakeTerminal) DarkBackground() bool  { return true }

type fakeService struct {
	res scan.Result
	err error
	// events are sent to the observer before returning.
	events bool
	// migrator plans and applies migrations; the zero value cannot.
	migrator app.Migrator
}

func (f fakeService) Scan(_ context.Context, observe scan.Observer) (scan.Result, error) {
	if f.events && observe != nil {
		for _, stage := range scan.Stages {
			observe(scan.Event{Stage: stage, Status: scan.StatusRunning})
			observe(scan.Event{Stage: stage, Status: scan.StatusDone})
		}
	}
	return f.res, f.err
}

func (f fakeService) Assess(ctx context.Context, observe scan.Observer) (app.Assessment, error) {
	svc, err := f.service()
	if err != nil {
		return app.Assessment{}, err
	}
	return svc.Assess(ctx, observe)
}

func (f fakeService) service() (*app.Service, error) {
	cat, err := catalog.Default()
	if err != nil {
		return nil, err
	}
	return app.New(scannerFunc(f.Scan), cat, wiring.Harnesses(), f.migrator)
}

func (f fakeService) PlanMigration(a app.Assessment, c app.MigrationChoice) (migrate.Plan, error) {
	svc, err := f.service()
	if err != nil {
		return migrate.Plan{}, err
	}
	return svc.PlanMigration(a, c)
}

func (f fakeService) PlanTry(a app.Assessment, c app.MigrationChoice) (migrate.Plan, error) {
	svc, err := f.service()
	if err != nil {
		return migrate.Plan{}, err
	}
	return svc.PlanTry(a, c)
}

func (f fakeService) SessionReport(ctx context.Context, a app.Assessment, p migrate.Plan, started, ended time.Time, exit int) (app.SessionReport, error) {
	svc, err := f.service()
	if err != nil {
		return app.SessionReport{}, err
	}
	return svc.SessionReport(ctx, a, p, started, ended, exit)
}

func (f fakeService) Inventory(a app.Assessment) (app.Inventory, error) {
	svc, err := f.service()
	if err != nil {
		return app.Inventory{}, err
	}
	return svc.Inventory(a)
}

func (f fakeService) Harnesses(ctx context.Context, a app.Assessment) []app.HarnessView {
	svc, err := f.service()
	if err != nil {
		return nil
	}
	return svc.Harnesses(ctx, a)
}

func (f fakeService) PlanInstall(a app.Assessment, id harnesspkg.ID) (migrate.Plan, error) {
	svc, err := f.service()
	if err != nil {
		return migrate.Plan{}, err
	}
	return svc.PlanInstall(a, id)
}

func (f fakeService) PlanRemove(a app.Assessment, rt runtimes.ID, model string) (migrate.Plan, error) {
	svc, err := f.service()
	if err != nil {
		return migrate.Plan{}, err
	}
	return svc.PlanRemove(a, rt, model)
}

func (f fakeService) PlanPull(a app.Assessment, c app.MigrationChoice) (migrate.Plan, error) {
	svc, err := f.service()
	if err != nil {
		return migrate.Plan{}, err
	}
	return svc.PlanPull(a, c)
}

func (f fakeService) PlanBenchmark(a app.Assessment, model string) (migrate.Plan, error) {
	svc, err := f.service()
	if err != nil {
		return migrate.Plan{}, err
	}
	return svc.PlanBenchmark(a, model)
}

func (f fakeService) PlanUndo() (migrate.Plan, error) {
	svc, err := f.service()
	if err != nil {
		return migrate.Plan{}, err
	}
	return svc.PlanUndo()
}

func (f fakeService) Migrate(ctx context.Context, p migrate.Plan, observe migrate.Observer) (migrate.Outcome, error) {
	svc, err := f.service()
	if err != nil {
		return migrate.Outcome{}, err
	}
	return svc.Migrate(ctx, p, observe)
}

type scannerFunc func(context.Context, scan.Observer) (scan.Result, error)

func (f scannerFunc) Run(ctx context.Context, observe scan.Observer) (scan.Result, error) {
	return f(ctx, observe)
}

type harness struct {
	app      *cli.App
	out, err *bytes.Buffer
	env      wiring.Environment
}

func newHarness(svc cli.Service) *harness {
	h := &harness{out: &bytes.Buffer{}, err: &bytes.Buffer{}}
	h.app = &cli.App{
		In:      strings.NewReader(""),
		Out:     h.out,
		Err:     h.err,
		Term:    fakeTerminal{},
		Version: "v0.0.1-test",
		Env: wiring.Environment{
			GOOS:   "linux",
			Home:   "/home/u",
			Getenv: func(string) string { return "" },
		},
		NewService: func(env wiring.Environment) (cli.Service, error) {
			h.env = env
			return svc, nil
		},
	}
	return h
}

func result() scan.Result {
	return scan.Result{
		GeneratedAt: time.Date(2026, 11, 4, 18, 0, 0, 0, time.UTC),
		Machine: machine.Profile{
			CPU:    machine.CPU{Model: "Test CPU"},
			Memory: machine.Memory{TotalBytes: 64 << 30, Unified: true},
			GPUs:   []machine.GPU{{Name: "Apple M5 Pro", Vendor: machine.VendorApple, Cores: 20, Unified: true}},
		},
		Usage: usage.Profile{Sessions: 2, Turns: 9, Context: usage.Context{P50: 40_000, P90: 90_000, Peaks: []int{40_000, 90_000}}},
	}
}

func TestScanPrintsPlainReportWhenNotATerminal(t *testing.T) {
	h := newHarness(fakeService{res: result()})
	if code := h.app.Run(t.Context(), []string{"scan"}); code != cli.ExitOK {
		t.Fatalf("exit %d, stderr: %s", code, h.err)
	}
	out := h.out.String()
	if !strings.Contains(out, "Test CPU") || !strings.Contains(out, "Session history") {
		t.Errorf("report missing sections:\n%s", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Error("plain output contains escape sequences")
	}
	if h.err.Len() != 0 {
		t.Errorf("stderr = %q, want nothing without a terminal", h.err)
	}
}

func TestScanJSON(t *testing.T) {
	h := newHarness(fakeService{res: result()})
	if code := h.app.Run(t.Context(), []string{"scan", "--json"}); code != cli.ExitOK {
		t.Fatalf("exit %d, stderr: %s", code, h.err)
	}
	var got struct {
		Schema  string `json:"schema"`
		Version string `json:"lo0Version"`
		Result  struct {
			Machine struct {
				CPU struct {
					Model string `json:"model"`
				} `json:"cpu"`
			} `json:"machine"`
			Usage struct {
				Sessions int `json:"sessions"`
			} `json:"usage"`
		} `json:"result"`
	}
	if err := json.Unmarshal(h.out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, h.out)
	}
	if got.Schema != cli.ScanSchema || got.Version != "v0.0.1-test" || got.Result.Machine.CPU.Model != "Test CPU" || got.Result.Usage.Sessions != 2 {
		t.Errorf("JSON = %+v", got)
	}
}

func TestMatchReport(t *testing.T) {
	h := newHarness(fakeService{res: result()})
	if code := h.app.Run(t.Context(), []string{"match"}); code != cli.ExitOK {
		t.Fatalf("exit %d, stderr: %s", code, h.err)
	}
	for _, want := range []string{"lo0 match", "Verdict", "Models for this machine", "Carries over to Claude Code on a local model"} {
		if !strings.Contains(h.out.String(), want) {
			t.Errorf("match report missing %q:\n%s", want, h.out)
		}
	}
}

func TestMatchJSON(t *testing.T) {
	h := newHarness(fakeService{res: result()})
	if code := h.app.Run(t.Context(), []string{"match", "--json"}); code != cli.ExitOK {
		t.Fatalf("exit %d, stderr: %s", code, h.err)
	}
	var got struct {
		Schema string `json:"schema"`
		Result struct {
			Match struct {
				Verdict struct {
					Level string `json:"level"`
				} `json:"verdict"`
				Candidates []json.RawMessage `json:"candidates"`
			} `json:"match"`
		} `json:"result"`
	}
	if err := json.Unmarshal(h.out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if got.Schema != cli.MatchSchema || got.Result.Match.Verdict.Level == "" || len(got.Result.Match.Candidates) == 0 {
		t.Errorf("match JSON = %+v", got)
	}
}

func TestScanClaudeDirOverride(t *testing.T) {
	h := newHarness(fakeService{res: result()})
	if code := h.app.Run(t.Context(), []string{"scan", "--claude-dir", "/tmp/claude"}); code != cli.ExitOK {
		t.Fatalf("exit %d, stderr: %s", code, h.err)
	}
	if got := h.env.Getenv("CLAUDE_CONFIG_DIR"); got != "/tmp/claude" {
		t.Errorf("CLAUDE_CONFIG_DIR = %q, want /tmp/claude", got)
	}
	if got := h.env.Getenv("HOME"); got != "" {
		t.Errorf("override leaked into other variables: HOME = %q", got)
	}
}

func TestScanCodexDirOverride(t *testing.T) {
	h := newHarness(fakeService{res: result()})
	if code := h.app.Run(t.Context(), []string{"scan", "--codex-dir", "/tmp/codex"}); code != cli.ExitOK {
		t.Fatalf("exit %d, stderr: %s", code, h.err)
	}
	if got := h.env.Getenv(codex.HomeVar); got != "/tmp/codex" {
		t.Errorf("%s = %q, want /tmp/codex", codex.HomeVar, got)
	}
}

func TestExitCodes(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		scanErr error
		want    int
		stderr  string
		stdout  string
	}{
		{name: "no arguments", args: nil, want: cli.ExitUsage, stdout: "Commands"},
		{name: "help", args: []string{"--help"}, want: cli.ExitOK, stdout: "scan"},
		{name: "unknown command", args: []string{"migrate-everything"}, want: cli.ExitUsage, stderr: `unknown command "migrate-everything"`},
		{name: "bad flag", args: []string{"scan", "--nope"}, want: cli.ExitUsage, stderr: "flag provided but not defined"},
		{name: "stray argument", args: []string{"scan", "extra"}, want: cli.ExitUsage, stderr: `unexpected argument "extra"`},
		{name: "scan help", args: []string{"scan", "-h"}, want: cli.ExitOK, stderr: "--claude-dir"},
		{name: "scan help names --codex-dir", args: []string{"scan", "-h"}, want: cli.ExitOK, stderr: "--codex-dir"},
		{name: "scan failure", args: []string{"scan"}, scanErr: errors.New("disk on fire"), want: cli.ExitFailure, stderr: "lo0 scan: disk on fire"},
		{name: "interrupted", args: []string{"scan"}, scanErr: context.Canceled, want: cli.ExitInterrupted, stderr: "Nothing was changed"},
		{name: "version", args: []string{"version"}, want: cli.ExitOK, stdout: "lo0 v0.0.1-test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(fakeService{res: result(), err: tt.scanErr})
			if code := h.app.Run(t.Context(), tt.args); code != tt.want {
				t.Errorf("exit = %d, want %d (stderr: %s)", code, tt.want, h.err)
			}
			if tt.stderr != "" && !strings.Contains(h.err.String(), tt.stderr) {
				t.Errorf("stderr = %q, want it to contain %q", h.err, tt.stderr)
			}
			if tt.stdout != "" && !strings.Contains(h.out.String(), tt.stdout) {
				t.Errorf("stdout = %q, want it to contain %q", h.out, tt.stdout)
			}
		})
	}
}

func TestSystemServiceBuilds(t *testing.T) {
	env := wiring.Environment{GOOS: "linux", Home: t.TempDir(), Getenv: func(string) string { return "" }}
	s, err := cli.SystemService(env)
	if err != nil || s == nil {
		t.Fatalf("SystemService = %v, %v", s, err)
	}
}

func TestScanShowsProgressOnTerminal(t *testing.T) {
	h := newHarness(fakeService{res: result(), events: true})
	h.app.Term = fakeTerminal{tty: true}
	if code := h.app.Run(t.Context(), []string{"scan"}); code != cli.ExitOK {
		t.Fatalf("exit %d, stderr: %s", code, h.err)
	}
	const hideCursor = "\x1b[?25l"
	if !strings.Contains(h.err.String(), hideCursor) {
		t.Errorf("progress display did not run on stderr: %q", h.err)
	}
	if !strings.Contains(h.out.String(), "Test CPU") {
		t.Errorf("no report on stdout: %q", h.out)
	}
	if strings.Contains(h.out.String(), hideCursor) {
		t.Error("progress display wrote to stdout")
	}
}

func TestUIRequiresATerminal(t *testing.T) {
	h := newHarness(fakeService{res: result()})
	if code := h.app.Run(t.Context(), []string{"ui"}); code != cli.ExitFailure {
		t.Fatalf("exit = %d, want failure without a terminal", code)
	}
	if !strings.Contains(h.err.String(), "needs a terminal") {
		t.Errorf("stderr = %q", h.err)
	}
}

func TestNoArgumentsWithoutTerminalPrintsUsage(t *testing.T) {
	h := newHarness(fakeService{res: result()})
	if code := h.app.Run(t.Context(), nil); code != cli.ExitUsage || !strings.Contains(h.out.String(), "ui ") {
		t.Errorf("exit %d, stdout %q; want usage listing the ui command", code, h.out)
	}
}

func TestOfflineSkipsTheReleaseLookup(t *testing.T) {
	h := newHarness(fakeService{res: result()})
	if code := h.app.Run(t.Context(), []string{"scan", "--offline"}); code != cli.ExitOK {
		t.Fatalf("exit %d, stderr: %s", code, h.err)
	}
	if h.env.Getenv(wiring.OfflineVar) == "" {
		t.Errorf("%s not set", wiring.OfflineVar)
	}
}

func TestRedactRemovesProjectPaths(t *testing.T) {
	res := result()
	res.Usage.Repos = []usage.Repo{{Path: "/home/u/clients/acme", Sessions: 2, ContextP90: 90_000}}
	res.Usage.Records = []usage.SessionRecord{{Repo: "/home/u/clients/acme", PeakContext: 40_000}}
	res.Layout.ConfigDir = "/home/u/.claude"
	for _, args := range [][]string{{"scan", "--redact"}, {"scan", "--redact", "--json"}, {"match", "--redact"}, {"match", "--redact", "--json"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			h := newHarness(fakeService{res: res})
			if code := h.app.Run(t.Context(), args); code != cli.ExitOK {
				t.Fatalf("exit %d, stderr: %s", code, h.err)
			}
			out := h.out.String()
			if strings.Contains(out, "acme") || strings.Contains(out, "/home/u") {
				t.Errorf("output still names the project or home directory:\n%s", out)
			}
			if !strings.Contains(out, "project 1") {
				t.Errorf("output does not say which project:\n%s", out)
			}
		})
	}
	h := newHarness(fakeService{res: res})
	if h.app.Run(t.Context(), []string{"scan"}); !strings.Contains(filepath.ToSlash(h.out.String()), "~/clients/acme") {
		t.Errorf("without --redact the project path should show:\n%s", h.out)
	}
}
