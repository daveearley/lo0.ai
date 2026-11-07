package opencode

import (
	"cmp"
	"context"
	"encoding/json"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/system"
)

// installScript is OpenCode 2's installer for macOS and Linux, and
// npmPackage its npm package, which installs the native binary for the
// platform it runs on (https://opencode.ai/v2/docs). The docs name no
// Windows package manager, so on Windows the user runs npm or downloads the
// build. lo0 installs OpenCode 2 only; an OpenCode 1 that is already
// installed is left as it is and planned for as it is.
const (
	installScript = "curl -fsSL https://opencode.ai/v2/install | bash"
	npmPackage    = "@opencode/cli"
)

// versionPattern finds the release number in opencode --version output.
var versionPattern = regexp.MustCompile(`\d+\.\d+\.\d+\S*`)

// Status implements harness reporting: whether OpenCode is installed, its
// version and the model its global configuration starts with. It only
// reads.
func (Harness) Status(ctx context.Context, req migrate.Request, files migrate.Files, runner system.Runner) harness.Installation {
	var in harness.Installation
	if req.LookPath == nil || !found(req.LookPath, Program) {
		return in
	}
	in.Installed = true
	if out, err := runner.Run(ctx, Program, "--version"); err == nil {
		in.Version = versionPattern.FindString(string(out))
	}
	use := harness.Use{Command: Program, Model: "its default model"}
	if data, err := files.ReadFile(filepath.Join(configDir(req), "opencode.json")); err == nil {
		use = configured(data)
	}
	in.Uses = []harness.Use{use}
	return in
}

// providerConfig is a provider entry in either release's form: OpenCode 1
// puts the address under options, OpenCode 2 under settings.
type providerConfig struct {
	Name    string `json:"name"`
	Options struct {
		BaseURL string `json:"baseURL"`
	} `json:"options"`
	Settings struct {
		BaseURL string `json:"baseURL"`
	} `json:"settings"`
}

// configured reads the default model from an opencode.json and names it
// with its provider, saying whether the provider is on this machine. An
// OpenCode 2 "providers" entry wins over an OpenCode 1 "provider" entry of
// the same name, as it does in OpenCode 2.
func configured(data []byte) harness.Use {
	use := harness.Use{Command: Program, Model: "its default model"}
	var cfg struct {
		Model     string                    `json:"model"`
		Provider  map[string]providerConfig `json:"provider"`
		Providers map[string]providerConfig `json:"providers"`
	}
	if json.Unmarshal(data, &cfg) != nil || cfg.Model == "" {
		return use
	}
	use.Model = cfg.Model
	provider, model, ok := strings.Cut(cfg.Model, "/")
	p, has := cfg.Providers[provider]
	if !has {
		p, has = cfg.Provider[provider]
	}
	if ok && has {
		if p.Name != "" {
			use.Model = model + " through " + strings.TrimSuffix(p.Name, " (local)")
		}
		if u, err := url.Parse(cmp.Or(p.Settings.BaseURL, p.Options.BaseURL)); err == nil {
			host := u.Hostname()
			use.Local = host == "127.0.0.1" || host == "localhost" || host == "::1"
		}
	}
	return use
}

// Install implements harness installing: the step that runs OpenCode's own
// installer, shown as it will run. On Windows it is the user's to run.
func (Harness) Install(req migrate.Request) migrate.Step {
	if req.GOOS == "windows" {
		s := install()
		s.Detail = []string{"Run: npm install -g " + npmPackage, "Or download the Windows build linked from https://opencode.ai/v2/docs"}
		return s
	}
	return migrate.Step{
		Kind:   migrate.KindInstall,
		Title:  "Install OpenCode",
		Detail: []string{installScript, "OpenCode's own installer. lo0 migrate --undo does not uninstall it."},
		Run: func(ctx context.Context, fx migrate.Effects, progress func(done, total int64)) error {
			if _, err := fx.Runner.Run(ctx, "bash", "-o", "pipefail", "-c", installScript); err != nil {
				return err
			}
			progress(1, 1)
			return nil
		},
	}
}
