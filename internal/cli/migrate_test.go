package cli_test

import (
	"encoding/json"
	"strings"
	"testing"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/backend"
	"lo0.ai/lo0/internal/cli"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/migrate/migratetest"
	"lo0.ai/lo0/internal/system/systemtest"
)

const journal = "/home/u/.local/state/lo0/migrations.json"

func migrating(files *migratetest.Files) fakeService {
	runner := &systemtest.Runner{}
	return fakeService{res: result(), migrator: app.Migrator{
		Backends: []migrate.Backend{backend.Ollama{}, backend.LMStudio{}, backend.LlamaCpp{}},
		GOOS:     "linux", Home: "/home/u",
		Getenv:   func(string) string { return "" },
		LookPath: runner.LookPath,
		Effects:  migrate.Effects{Files: files, Open: files.Open, Runner: runner, Now: migratetest.Clock, Journal: migrate.Journal{Path: journal}},
	}}
}

// The hint names the plan that was shown: without the flags that chose it,
// running the hint would apply the default plan without asking.
func TestMigrateHintKeepsTheChoice(t *testing.T) {
	files := migratetest.NewFiles(nil)
	h := newHarness(migrating(files))
	if code := h.app.Run(t.Context(), []string{"migrate", "--to", "opencode", "--runtime", "ollama", "--offline"}); code != cli.ExitOK {
		t.Fatalf("exit %d, stderr: %s", code, h.err)
	}
	if want := "lo0 migrate --to opencode --runtime ollama --offline --yes"; !strings.Contains(h.err.String(), want) {
		t.Errorf("stderr = %q, want %q", h.err, want)
	}
}

func TestMigrateShowsThePlanAndChangesNothingWithoutYes(t *testing.T) {
	files := migratetest.NewFiles(nil)
	h := newHarness(migrating(files))
	if code := h.app.Run(t.Context(), []string{"migrate"}); code != cli.ExitOK {
		t.Fatalf("exit %d, stderr: %s", code, h.err)
	}
	for _, want := range []string{"lo0 migrate", "Claude Code on a local model", "Install Ollama", "claude-local", "Then"} {
		if !strings.Contains(h.out.String(), want) {
			t.Errorf("plan lacks %q:\n%s", want, h.out)
		}
	}
	if !strings.Contains(h.err.String(), "lo0 migrate --yes") {
		t.Errorf("stderr = %q, want how to apply", h.err)
	}
	if len(files.Names()) != 0 {
		t.Errorf("files were written without confirmation: %v", files.Names())
	}
}

func TestMigrateAsksOnATerminal(t *testing.T) {
	for _, tt := range []struct {
		answer string
		writes bool
	}{{"y\n", true}, {"\n", false}, {"no\n", false}} {
		files := migratetest.NewFiles(nil)
		h := newHarness(migrating(files))
		// Progress on stderr would read the answer from stdin itself.
		h.app.In = strings.NewReader(tt.answer)
		h.app.Term = ttyExcept{h.err}
		h.app.Run(t.Context(), []string{"migrate", "--to", "claude"})
		_, wrote := files.Get("/home/u/.local/bin/claude-local")
		if wrote != tt.writes {
			t.Errorf("answer %q: wrote = %v, want %v (stderr %q)", tt.answer, wrote, tt.writes, h.err)
		}
		if tt.writes && !strings.Contains(h.out.String(), "Still for you to do") {
			t.Errorf("outcome does not list what is left:\n%s", h.out)
		}
	}
}

func TestMigrateYesApplies(t *testing.T) {
	files := migratetest.NewFiles(nil)
	h := newHarness(migrating(files))
	if code := h.app.Run(t.Context(), []string{"migrate", "--to", "opencode", "--yes"}); code != cli.ExitOK {
		t.Fatalf("exit %d, stderr: %s", code, h.err)
	}
	if _, ok := files.Get("/home/u/.config/opencode/opencode.json"); !ok {
		t.Errorf("OpenCode was not configured: %v", files.Names())
	}
	if strings.Contains(h.err.String(), "\x1b[") {
		t.Errorf("stderr has escape sequences although it is not a terminal: %q", h.err)
	}
	if !strings.Contains(h.err.String(), "Add ") {
		t.Errorf("no step progress on stderr: %q", h.err)
	}
}

func TestMigrateDryRunAndJSON(t *testing.T) {
	files := migratetest.NewFiles(nil)
	h := newHarness(migrating(files))
	if code := h.app.Run(t.Context(), []string{"migrate", "--json", "--yes", "--dry-run"}); code != cli.ExitOK {
		t.Fatalf("exit %d, stderr: %s", code, h.err)
	}
	var got struct {
		Schema string `json:"schema"`
		Result struct {
			Plan    migrate.Plan     `json:"plan"`
			Outcome *migrate.Outcome `json:"outcome"`
		} `json:"result"`
	}
	if err := json.Unmarshal(h.out.Bytes(), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, h.out)
	}
	if got.Schema != cli.MigrateSchema || len(got.Result.Plan.Steps) == 0 || got.Result.Outcome != nil {
		t.Errorf("JSON = %+v", got)
	}
	if len(files.Names()) != 0 {
		t.Errorf("a dry run wrote %v", files.Names())
	}
}

func TestMigrateRejectsUnknownChoices(t *testing.T) {
	for _, args := range [][]string{{"migrate", "--to", "aider"}, {"migrate", "--runtime", "vllm"}} {
		h := newHarness(migrating(migratetest.NewFiles(nil)))
		if code := h.app.Run(t.Context(), args); code != cli.ExitUsage {
			t.Errorf("%v: exit %d, want usage", args, code)
		}
		if !strings.Contains(h.err.String(), "unknown") {
			t.Errorf("%v: stderr = %q", args, h.err)
		}
	}
}

// ttyExcept is a terminal on every stream but one.
type ttyExcept struct{ not any }

func (t ttyExcept) IsTerminal(s any) bool { return s != t.not }
func (ttyExcept) Width(any) int           { return 80 }
func (ttyExcept) DarkBackground() bool    { return true }

func TestMigrateUndoPutsEverythingBack(t *testing.T) {
	files := migratetest.NewFiles(map[string]string{"/home/u/.config/opencode/opencode.json": `{"theme":"mine"}`})
	for _, to := range []string{"claude", "opencode"} {
		if code := newHarness(migrating(files)).app.Run(t.Context(), []string{"migrate", "--to", to, "--yes"}); code != cli.ExitOK {
			t.Fatalf("migrate --to %s: exit %d", to, code)
		}
	}
	if _, ok := files.Get(journal); !ok {
		t.Fatal("migrate kept no journal")
	}

	h := newHarness(migrating(files))
	if code := h.app.Run(t.Context(), []string{"migrate", "--undo"}); code != cli.ExitOK {
		t.Fatalf("exit %d, stderr: %s", code, h.err)
	}
	for _, want := range []string{"lo0 migrate --undo", "Restore opencode.json", "~/.config/opencode/opencode.json", "Delete claude-local", "~/.local/bin/claude-local"} {
		if !strings.Contains(h.out.String(), want) {
			t.Errorf("undo plan lacks %q:\n%s", want, h.out)
		}
	}
	if strings.Contains(h.err.String(), "\x1b[") {
		t.Errorf("stderr has escape sequences although it is not a terminal: %q", h.err)
	}
	if !strings.Contains(h.err.String(), "lo0 migrate --undo --yes") {
		t.Errorf("stderr = %q, want how to apply the undo", h.err)
	}

	h = newHarness(migrating(files))
	if code := h.app.Run(t.Context(), []string{"migrate", "--undo", "--yes"}); code != cli.ExitOK {
		t.Fatalf("exit %d, stderr: %s", code, h.err)
	}
	if names := files.Names(); len(names) != 1 {
		t.Errorf("files left = %v, want only the user's opencode.json", names)
	}
	if got, _ := files.Get("/home/u/.config/opencode/opencode.json"); got != `{"theme":"mine"}` {
		t.Errorf("opencode.json = %q", got)
	}

	h = newHarness(migrating(files))
	h.app.Run(t.Context(), []string{"migrate", "--undo"})
	if !strings.Contains(h.out.String(), "nothing to undo") {
		t.Errorf("second undo said %q", h.out)
	}
	if code := newHarness(migrating(files)).app.Run(t.Context(), []string{"migrate", "--undo", "--to", "opencode"}); code != cli.ExitUsage {
		t.Errorf("--undo with --to: exit %d, want usage", code)
	}
}
