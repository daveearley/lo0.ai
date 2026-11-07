package claude_test

import (
	"strings"
	"testing"

	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/harness/claude"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/migrate/migratetest"
	"lo0.ai/lo0/internal/system/systemtest"
)

func TestStatusReadsTheVersionAndTheLauncher(t *testing.T) {
	runner := &systemtest.Runner{
		Paths:    map[string]string{"claude": "/Users/u/.local/bin/claude"},
		Commands: map[string]systemtest.Result{"claude --version": {Output: "2.1.221 (Claude Code)\n"}},
	}
	req := migrate.Request{GOOS: "darwin", Home: "/Users/u", LookPath: runner.LookPath}
	set, err := claude.Harness{}.Plan(req, served())
	if err != nil {
		t.Fatal(err)
	}
	files := migratetest.NewFiles(nil)
	if err := set.Steps[0].Run(t.Context(), migrate.Effects{Files: files, Now: migratetest.Clock}, func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	got := claude.Harness{}.Status(t.Context(), req, files, runner)
	want := []harness.Use{
		{Command: "claude", Model: "Claude"},
		{Command: "claude-local", Model: "Qwen3-Coder-30B-A3B through Ollama", Local: true},
	}
	if !got.Installed || got.Version != "2.1.221" || len(got.Uses) != 2 || got.Uses[0] != want[0] || got.Uses[1] != want[1] {
		t.Errorf("status = %+v, want installed 2.1.221 with %+v", got, want)
	}
}

func TestStatusWhenMissing(t *testing.T) {
	runner := &systemtest.Runner{}
	got := claude.Harness{}.Status(t.Context(), migrate.Request{GOOS: "linux", Home: "/home/u", LookPath: runner.LookPath}, migratetest.NewFiles(nil), runner)
	if got.Installed || len(got.Uses) != 0 {
		t.Errorf("status = %+v, want not installed", got)
	}
}

func TestInstallRunsTheDocumentedInstaller(t *testing.T) {
	step := claude.Harness{}.Install(migrate.Request{GOOS: "darwin"})
	if step.Kind != migrate.KindInstall || step.Detail[0] != "curl -fsSL https://claude.ai/install.sh | bash" {
		t.Fatalf("step = %+v", step)
	}
	runner := &systemtest.Runner{Commands: map[string]systemtest.Result{"bash -o pipefail -c curl -fsSL https://claude.ai/install.sh | bash": {}}}
	if err := step.Run(t.Context(), migrate.Effects{Runner: runner}, func(int64, int64) {}); err != nil {
		t.Errorf("run: %v (calls %q)", err, runner.Calls())
	}
	win := claude.Harness{}.Install(migrate.Request{GOOS: "windows"})
	if win.Kind != migrate.KindManual || !strings.Contains(win.Detail[0], "install.ps1") {
		t.Errorf("windows step = %+v, want the PowerShell installer for the user to run", win)
	}
}
