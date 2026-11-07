package codex

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"

	codexhome "lo0.ai/lo0/internal/codex"
	codexsetup "lo0.ai/lo0/internal/codex/setup"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/system"
)

// Codex's documented installers: a script for macOS and Linux, one for
// PowerShell on Windows, and its npm package.
const (
	installScript     = "curl -fsSL https://chatgpt.com/codex/install.sh | sh"
	installPowerShell = "irm https://chatgpt.com/codex/install.ps1 | iex"
	npmPackage        = "@openai/codex"
)

// versionPattern finds the release number in codex --version output, such
// as "codex-cli 0.167.0".
var versionPattern = regexp.MustCompile(`\d+\.\d+\.\d+\S*`)

// builtIn are the providers Codex has without configuration that run on
// this machine.
var builtIn = map[string]string{"ollama": "Ollama", "lmstudio": "LM Studio"}

// Status implements harness reporting: whether Codex is installed, its
// version and the model config.toml starts it with. It only reads.
func (Harness) Status(ctx context.Context, req migrate.Request, files migrate.Files, runner system.Runner) harness.Installation {
	var in harness.Installation
	if !installed(req) {
		return in
	}
	in.Installed = true
	if out, err := runner.Run(ctx, Program, "--version"); err == nil {
		in.Version = versionPattern.FindString(string(out))
	}
	use := harness.Use{Command: Program, Model: "OpenAI"}
	path := filepath.Join(codexhome.Home(req.Home, getenv(req)), codexsetup.ConfigFile)
	if data, err := files.ReadFile(path); err == nil {
		use = configured(data)
	}
	in.Uses = []harness.Use{use}
	return in
}

// configured names the model a config.toml starts Codex with and its
// provider, saying whether the provider is on this machine.
func configured(data []byte) harness.Use {
	use := harness.Use{Command: Program, Model: "OpenAI"}
	var cfg struct {
		Model          string `toml:"model"`
		ModelProvider  string `toml:"model_provider"`
		ModelProviders map[string]struct {
			Name    string `toml:"name"`
			BaseURL string `toml:"base_url"`
		} `toml:"model_providers"`
	}
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return use
	}
	if cfg.Model != "" {
		use.Model = cfg.Model
	}
	if name, ok := builtIn[cfg.ModelProvider]; ok {
		use.Model, use.Local = use.Model+" through "+name, true
		return use
	}
	if p, ok := cfg.ModelProviders[cfg.ModelProvider]; ok {
		if p.Name != "" && cfg.Model != "" {
			use.Model = cfg.Model + " through " + strings.TrimSuffix(p.Name, " (local)")
		}
		use.Local = codexsetup.IsLocal(p.BaseURL)
	}
	return use
}

func installed(req migrate.Request) bool {
	if req.LookPath == nil {
		return false
	}
	_, err := req.LookPath(Program)
	return err == nil
}

// prepare is the step Codex needs before it can start, if any: an install
// when it is not on PATH, a reinstall when it is there but did not answer
// codex --version.
func prepare(req migrate.Request) []migrate.Step {
	switch {
	case !installed(req):
		return []migrate.Step{install()}
	case req.Target.Installed && req.Target.Version == "":
		return []migrate.Step{reinstall()}
	}
	return nil
}

func reinstall() migrate.Step {
	return migrate.Step{
		Kind:   migrate.KindManual,
		Title:  "Reinstall Codex",
		Detail: []string{"codex is on PATH but codex --version fails, so it cannot start.", "Run: " + installScript, "Or: npm install -g " + npmPackage},
		Run: func(ctx context.Context, fx migrate.Effects, _ func(done, total int64)) error {
			_, err := fx.Runner.Run(ctx, Program, "--version")
			return err
		},
	}
}

func install() migrate.Step {
	return migrate.Step{
		Kind:   migrate.KindManual,
		Title:  "Install Codex",
		Detail: []string{"Run: " + installScript, "Or: npm install -g " + npmPackage},
		Run: func(_ context.Context, fx migrate.Effects, _ func(done, total int64)) error {
			_, err := fx.Runner.LookPath(Program)
			return err
		},
	}
}

// Install implements harness installing: the step that runs Codex's own
// installer, shown as it will run. On Windows it is the user's to run.
func (Harness) Install(req migrate.Request) migrate.Step {
	if req.GOOS == "windows" {
		s := install()
		s.Detail = []string{"Run in PowerShell: " + installPowerShell, "Or: npm install -g " + npmPackage}
		return s
	}
	return migrate.Step{
		Kind:   migrate.KindInstall,
		Title:  "Install Codex",
		Detail: []string{installScript, "Codex's own installer. lo0 migrate --undo does not uninstall it."},
		Run: func(ctx context.Context, fx migrate.Effects, progress func(done, total int64)) error {
			if _, err := fx.Runner.Run(ctx, "bash", "-o", "pipefail", "-c", installScript); err != nil {
				return err
			}
			progress(1, 1)
			return nil
		},
	}
}
