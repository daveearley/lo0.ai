package claude

import (
	"context"
	"path/filepath"
	"strings"

	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/system"
)

// installScript is Claude Code's documented installer for macOS and Linux,
// and installPowerShell the one for Windows.
const (
	installScript     = "curl -fsSL https://claude.ai/install.sh | bash"
	installPowerShell = "irm https://claude.ai/install.ps1 | iex"
)

// Status implements harness reporting: whether Claude Code is installed,
// its version, and what claude and lo0's claude-local launcher use. It
// only reads.
func (Harness) Status(ctx context.Context, req migrate.Request, files migrate.Files, runner system.Runner) harness.Installation {
	var in harness.Installation
	if installed(req) {
		in.Installed = true
		// claude --version prints "2.1.211 (Claude Code)".
		if out, err := runner.Run(ctx, Program, "--version"); err == nil {
			in.Version, _, _ = strings.Cut(strings.TrimSpace(string(out)), " ")
		}
		in.Uses = append(in.Uses, harness.Use{Command: Program, Model: "Claude"})
	}
	path := filepath.Join(binDir(req), Command)
	if req.GOOS == "windows" {
		path += ".cmd"
	}
	if data, err := files.ReadFile(path); err == nil {
		if model, ok := launcherModel(string(data)); ok {
			in.Uses = append(in.Uses, harness.Use{Command: Command, Model: model, Local: true})
		}
	}
	return in
}

// launcherModel reads what a launcher lo0 wrote runs, from its first
// comment: "Runs Claude Code with <model> through <runtime>."
func launcherModel(script string) (string, bool) {
	_, after, ok := strings.Cut(script, "Runs Claude Code with ")
	if !ok {
		return "", false
	}
	model, _, ok := strings.Cut(after, ". Delete this file")
	return model, ok && model != ""
}

// Install implements harness installing: the step that runs Claude Code's
// own installer, shown as it will run. On Windows it is the user's to run.
func (Harness) Install(req migrate.Request) migrate.Step {
	if req.GOOS == "windows" {
		s := install()
		s.Detail = []string{"Run in PowerShell: " + installPowerShell}
		return s
	}
	return migrate.Step{
		Kind:   migrate.KindInstall,
		Title:  "Install Claude Code",
		Detail: []string{installScript, "Claude Code's own installer. lo0 migrate --undo does not uninstall it."},
		Run: func(ctx context.Context, fx migrate.Effects, progress func(done, total int64)) error {
			if _, err := fx.Runner.Run(ctx, "bash", "-o", "pipefail", "-c", installScript); err != nil {
				return err
			}
			progress(1, 1)
			return nil
		},
	}
}
