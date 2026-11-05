package app_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/backend"
	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/migrate/migratetest"
	"lo0.ai/lo0/internal/runtimes"
	"lo0.ai/lo0/internal/scan"
	"lo0.ai/lo0/internal/system/systemtest"
)

func migrator(files *migratetest.Files) app.Migrator {
	runner := &systemtest.Runner{}
	return app.Migrator{
		Backends: []migrate.Backend{backend.Ollama{}, backend.LMStudio{}, backend.LlamaCpp{}},
		GOOS:     "linux", Home: "/home/u",
		Getenv:   func(string) string { return "" },
		LookPath: runner.LookPath,
		Effects:  migrate.Effects{Files: files, Open: files.Open, Runner: runner, Now: migratetest.Clock},
	}
}

func assess(t *testing.T, res scan.Result, mig app.Migrator) (*app.Service, app.Assessment) {
	t.Helper()
	cat, err := catalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	svc, err := app.New(&fakeScanner{res: res}, cat, harnesses(), mig)
	if err != nil {
		t.Fatal(err)
	}
	a, err := svc.Assess(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return svc, a
}

func TestPlanMigrationTakesTheRecommendation(t *testing.T) {
	svc, a := assess(t, result(), migrator(migratetest.NewFiles(nil)))
	p, err := svc.PlanMigration(a, app.MigrationChoice{})
	if err != nil {
		t.Fatal(err)
	}
	best, _ := a.Match.Best()
	if p.Harness != a.Match.TargetID || p.Model != best.Model.Name || p.Quant != best.Best.Quant.Name || p.Context != best.Best.MaxContext {
		t.Errorf("plan = %s %s %s %d, want the recommended harness and best fit", p.Harness, p.Model, p.Quant, p.Context)
	}
	if p.Runtime != runtimes.Ollama || p.Steps[0].Title != "Install Ollama" {
		t.Errorf("with no runtime installed, the plan starts by installing Ollama: %s, %q", p.Runtime, p.Steps[0].Title)
	}
}

func TestPlanMigrationFollowsTheInstalledHarness(t *testing.T) {
	for _, tt := range []struct {
		name, version, want string
	}{
		{"OpenCode 1", "1.19.5\n", `"provider.ollama"`},
		{"OpenCode 2", "2.2.5\n", `"providers.ollama"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mig := migrator(migratetest.NewFiles(nil))
			runner := &systemtest.Runner{
				Paths:    map[string]string{"opencode": "/usr/bin/opencode"},
				Commands: map[string]systemtest.Result{"opencode --version": {Output: tt.version}},
			}
			mig.LookPath, mig.Effects.Runner = runner.LookPath, runner
			svc, a := assess(t, result(), mig)
			if got := a.Scan.Harnesses[harness.OpenCode]; !got.Installed || got.Version+"\n" != tt.version {
				t.Fatalf("assessment records OpenCode as %+v", got)
			}
			p, err := svc.PlanMigration(a, app.MigrationChoice{Harness: harness.OpenCode})
			if err != nil {
				t.Fatal(err)
			}
			var detail []string
			for _, s := range p.Steps {
				detail = append(detail, s.Detail...)
			}
			if !slices.Contains(detail, tt.want+" at http://127.0.0.1:11434/v1") {
				t.Errorf("plan detail = %q, want %s", detail, tt.want)
			}
		})
	}
}

func TestPlanMigrationHonoursTheChoice(t *testing.T) {
	res := result()
	res.Runtimes = []runtimes.Runtime{
		{ID: runtimes.Ollama, Installed: true},
		{ID: runtimes.LMStudio, Installed: true, Running: true, Endpoint: runtimes.DefaultLMStudioURL},
	}
	svc, a := assess(t, res, migrator(migratetest.NewFiles(nil)))

	p, err := svc.PlanMigration(a, app.MigrationChoice{Harness: harness.OpenCode})
	if err != nil {
		t.Fatal(err)
	}
	if p.Harness != harness.OpenCode || p.Runtime != runtimes.LMStudio {
		t.Errorf("plan = %s on %s, want OpenCode on the running LM Studio", p.Harness, p.Runtime)
	}
	if p, _ := svc.PlanMigration(a, app.MigrationChoice{Runtime: runtimes.LlamaCpp}); p.Runtime != runtimes.LlamaCpp {
		t.Errorf("runtime = %s, want llama.cpp as asked", p.Runtime)
	}
	if _, err := svc.PlanMigration(a, app.MigrationChoice{Model: "no-such-model"}); err == nil || !strings.Contains(err.Error(), "no-such-model") {
		t.Errorf("an unknown model was planned: %v", err)
	}
	if _, err := svc.PlanMigration(a, app.MigrationChoice{Harness: "aider"}); err == nil {
		t.Error("an unknown harness was planned")
	}
}

func TestPlanMigrationNeedsAModelThatFits(t *testing.T) {
	res := result()
	res.Machine.Memory.TotalBytes = 1 << 30
	svc, a := assess(t, res, migrator(migratetest.NewFiles(nil)))
	if _, err := svc.PlanMigration(a, app.MigrationChoice{}); !errors.Is(err, app.ErrNoFit) {
		t.Errorf("err = %v, want ErrNoFit", err)
	}
}

func TestMigrateAppliesThePlan(t *testing.T) {
	files := migratetest.NewFiles(nil)
	svc, a := assess(t, result(), migrator(files))
	p, err := svc.PlanMigration(a, app.MigrationChoice{})
	if err != nil {
		t.Fatal(err)
	}
	out, err := svc.Migrate(t.Context(), p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Waiting()) != 1 {
		t.Errorf("states = %v, want installing Ollama left to the user", out.States)
	}
	if _, ok := files.Get("/home/u/.local/bin/claude-local"); !ok {
		t.Errorf("the launcher was not written: %v", files.Names())
	}
}

func TestMigrateNeedsAMigrator(t *testing.T) {
	svc, a := assess(t, result(), app.Migrator{})
	if _, err := svc.PlanMigration(a, app.MigrationChoice{}); err == nil {
		t.Error("a plan was made with no backends")
	}
	if _, err := svc.Migrate(t.Context(), migrate.Plan{}, nil); err == nil {
		t.Error("Migrate ran without a migrator")
	}
}

// fakeOllama answers what Apply and undo ask of Ollama and lists what it
// was asked to delete.
func fakeOllama(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var deleted []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/show":
			http.NotFound(w, r)
		case "/api/pull", "/api/create":
			fmt.Fprintln(w, `{"status":"success"}`)
		case "/api/delete":
			var body struct {
				Model string `json:"model"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			deleted = append(deleted, body.Model)
		case "/v1/messages":
			_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ready"}]}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &deleted
}

func TestMigrateAddsABackgroundModelAndUndoRemovesIt(t *testing.T) {
	srv, deleted := fakeOllama(t)
	res := result()
	res.Runtimes = []runtimes.Runtime{{ID: runtimes.Ollama, Installed: true, Running: true, Version: "0.21.0", Endpoint: srv.URL}}
	files := migratetest.NewFiles(nil)
	mig := migrator(files)
	mig.Effects.Client = srv.Client()
	mig.Effects.Journal = migrate.Journal{Path: "/home/u/.local/state/lo0/migrations.json"}
	svc, a := assess(t, res, mig)
	if a.Match.Background == nil {
		t.Fatal("no background model fits beside the best one on a 64 GiB machine")
	}
	bg := a.Match.Background

	p, err := svc.PlanMigration(a, app.MigrationChoice{Harness: harness.ClaudeCode})
	if err != nil {
		t.Fatal(err)
	}
	if p.Served.Background == nil || p.Served.Background.DisplayName != bg.Model.Name {
		t.Fatalf("served = %+v, want %s in the background", p.Served, bg.Model.Name)
	}
	var downloads []int64
	for _, s := range p.Steps {
		if s.Kind == migrate.KindDownload {
			downloads = append(downloads, s.Bytes)
		}
	}
	best, _ := a.Match.Best()
	if !slices.Equal(downloads, []int64{best.Best.Quant.Bytes, bg.Option.Quant.Bytes}) {
		t.Errorf("downloads = %v, want the main model's and the background model's sizes", downloads)
	}

	if _, err := svc.Migrate(t.Context(), p, nil); err != nil {
		t.Fatal(err)
	}
	script, _ := files.Get("/home/u/.local/bin/claude-local")
	if !strings.Contains(script, "export ANTHROPIC_DEFAULT_HAIKU_MODEL='"+p.Served.Background.Model+"'\n") {
		t.Errorf("launcher does not send haiku to %s:\n%s", p.Served.Background.Model, script)
	}

	undo, err := svc.PlanUndo()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Migrate(t.Context(), undo, nil); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{p.Served.Model, p.Served.Background.Model} {
		if !slices.Contains(*deleted, name) {
			t.Errorf("undo left %s in Ollama: deleted %q", name, *deleted)
		}
	}
	if len(*deleted) != 4 {
		t.Errorf("undo deleted %q, want both downloads and both models", *deleted)
	}
	if _, ok := files.Get("/home/u/.local/bin/claude-local"); ok {
		t.Error("undo left the launcher")
	}
}

func TestOnlyHarnessesThatUseItGetABackgroundModel(t *testing.T) {
	svc, a := assess(t, result(), migrator(migratetest.NewFiles(nil)))
	if a.Match.Background == nil {
		t.Fatal("no background model on a 64 GiB machine")
	}
	open, err := svc.PlanMigration(a, app.MigrationChoice{Harness: harness.OpenCode})
	if err != nil {
		t.Fatal(err)
	}
	pull, err := svc.PlanPull(a, app.MigrationChoice{Harness: harness.ClaudeCode})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []migrate.Plan{open, pull} {
		if p.Served.Background != nil {
			t.Errorf("%s plan for %s prepares a background model", p.Command, p.Harness)
		}
	}
	try, err := svc.PlanTry(a, app.MigrationChoice{Harness: harness.ClaudeCode})
	if err != nil {
		t.Fatal(err)
	}
	if try.Served.Background == nil || !slices.Contains(try.Session.Detail, "ANTHROPIC_DEFAULT_HAIKU_MODEL="+try.Served.Background.Model) {
		t.Errorf("try session = %+v, want haiku on the background model", try.Session)
	}
}
