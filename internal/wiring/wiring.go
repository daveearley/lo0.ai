// Package wiring builds lo0's services for the real machine. It is the one
// composition root shared by every front end, so the command line and the
// desktop app scan with exactly the same detectors.
package wiring

import (
	"cmp"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/backend"
	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/claudecode"
	"lo0.ai/lo0/internal/claudecode/history"
	"lo0.ai/lo0/internal/claudecode/install"
	"lo0.ai/lo0/internal/claudecode/setup"
	"lo0.ai/lo0/internal/codex"
	codexhistory "lo0.ai/lo0/internal/codex/history"
	"lo0.ai/lo0/internal/harness/claude"
	"lo0.ai/lo0/internal/harness/opencode"
	"lo0.ai/lo0/internal/machine"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/runtimes"
	"lo0.ai/lo0/internal/scan"
	"lo0.ai/lo0/internal/system"
	"lo0.ai/lo0/internal/usage"
)

const (
	commandTimeout = 10 * time.Second
	// probeTimeout bounds each local HTTP probe. Runtimes listen on loopback,
	// so anything slower means nothing useful is there.
	probeTimeout = 2 * time.Second
	// releaseTimeout bounds the one request lo0 makes off this machine: the
	// newest Claude Code version number, which sends nothing about the user.
	releaseTimeout = 3 * time.Second
)

// OfflineVar is the environment variable that, set to anything, stops lo0
// from looking up the newest Claude Code release.
const OfflineVar = "LO0_OFFLINE"

// Environment is what the process inherited.
type Environment struct {
	GOOS    string
	Home    string
	Getenv  func(string) string
	Environ []string
}

// WithOverride returns a copy of env in which key reads as value.
func (env Environment) WithOverride(key, value string) Environment {
	getenv := env.Getenv
	env.Getenv = func(k string) string {
		if k == key {
			return value
		}
		return getenv(k)
	}
	return env
}

// FromOS reads the real process environment.
func FromOS() (Environment, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Environment{}, errors.New("cannot find your home directory; set HOME and try again")
	}
	return Environment{GOOS: runtime.GOOS, Home: home, Getenv: os.Getenv, Environ: os.Environ()}, nil
}

// Scanner wires every scan stage to the real machine.
func Scanner(env Environment) (*scan.Scanner, error) {
	runner := system.ExecRunner{Timeout: commandTimeout}
	client := &http.Client{Timeout: probeTimeout}
	layout := claudecode.ResolveLayout(env.GOOS, env.Home, env.Getenv)
	open := os.DirFS
	claude := install.Detector{Runner: runner}
	if dir := install.AppDir(env.GOOS, env.Home); dir != "" {
		claude.App = open(dir)
	}
	if env.Getenv(OfflineVar) == "" {
		claude.Client, claude.RegistryURL = &http.Client{Timeout: releaseTimeout}, install.DefaultRegistryURL
	}

	return scan.New(scan.Sources{
		Layout:  layout,
		Machine: machine.NewProber(machine.SystemHost{}, machine.DefaultGPUProbes(env.GOOS, runner), env.Home),
		Runtimes: []scan.RuntimeDetector{
			runtimes.OllamaDetector{Runner: runner, Client: client, BaseURL: runtimes.OllamaBaseURL(env.Getenv("OLLAMA_HOST"))},
			runtimes.LMStudioDetector{Runner: runner, Client: client, BaseURL: runtimes.DefaultLMStudioURL, Models: open(runtimes.LMStudioModelsDir(env.Home))},
			runtimes.LlamaCppDetector{Runner: runner, Caches: openAll(runtimes.LlamaCppCacheDirs(env.GOOS, env.Home, env.Getenv))},
		},
		Setup:  setup.NewScanner(layout, open),
		Claude: claude,
		History: usage.Pool{
			Sources: []usage.Source{
				history.NewAnalyzer(open(layout.ProjectsDir()), runtime.GOMAXPROCS(0)),
				codexhistory.NewAnalyzer(open(codex.SessionsDir(codex.Home(env.Home, env.Getenv))), runtime.GOMAXPROCS(0)),
			},
			Repos: usage.NewGitRepos(claudecode.WorktreesDir),
		},
	})
}

// Service builds the application service for the real machine with the
// embedded model catalog.
func Service(env Environment) (*app.Service, error) {
	scanner, err := Scanner(env)
	if err != nil {
		return nil, err
	}
	cat, err := catalog.Default()
	if err != nil {
		return nil, err
	}
	return app.New(scanner, cat, Harnesses(), Migrator(env))
}

// Harnesses lists the harnesses lo0 can migrate to, in the order it prefers
// them when the same share of a setup carries over. Claude Code comes first
// because moving to it changes nothing the user already has.
func Harnesses() []app.Harness {
	return []app.Harness{claude.Harness{}, opencode.Harness{}}
}

// Migrator wires migrations to the real machine. Its runner and HTTP client
// have no timeouts of their own: downloads and model loads take as long as
// they take, and cancelling lo0 stops them.
func Migrator(env Environment) app.Migrator {
	runner := system.ExecRunner{}
	return app.Migrator{
		Backends: []migrate.Backend{backend.Ollama{}, backend.LMStudio{}, backend.LlamaCpp{}},
		GOOS:     env.GOOS,
		Home:     env.Home,
		Getenv:   env.Getenv,
		LookPath: runner.LookPath,
		Effects: migrate.Effects{
			Files:   migrate.OSFiles{},
			Open:    os.DirFS,
			Runner:  runner,
			Client:  &http.Client{},
			Now:     time.Now,
			Journal: migrate.Journal{Path: filepath.Join(StateDir(env), "migrations.json")},
			Lock:    migrate.LockFile,
		},
		Benchmarks: filepath.Join(StateDir(env), "benchmarks.json"),
		Swap:       machine.SwapMeter{GOOS: env.GOOS, Runner: system.ExecRunner{Timeout: commandTimeout}, Root: os.DirFS("/")},
		Histories: []app.RecentHistory{
			history.NewAnalyzer(os.DirFS(claudecode.ResolveLayout(env.GOOS, env.Home, env.Getenv).ProjectsDir()), runtime.GOMAXPROCS(0)),
		},
	}
}

// StateDir is where lo0 keeps what it needs between runs: the journal of
// changes it made, for undoing them, and its speed measurements. It follows
// each platform's convention for per-user application state.
func StateDir(env Environment) string {
	switch env.GOOS {
	case "darwin":
		return filepath.Join(env.Home, "Library", "Application Support", "lo0")
	case "windows":
		return filepath.Join(cmp.Or(env.Getenv("LOCALAPPDATA"), filepath.Join(env.Home, "AppData", "Local")), "lo0")
	default:
		return filepath.Join(cmp.Or(env.Getenv("XDG_STATE_HOME"), filepath.Join(env.Home, ".local", "state")), "lo0")
	}
}

// openAll opens each directory as a read-only file system.
func openAll(dirs []string) []fs.FS {
	out := make([]fs.FS, len(dirs))
	for i, d := range dirs {
		out[i] = os.DirFS(d)
	}
	return out
}
