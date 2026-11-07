package app_test

import (
	"slices"
	"strings"
	"testing"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/backend"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/migrate/migratetest"
	"lo0.ai/lo0/internal/runtimes"
)

const (
	journalPath = "/home/u/.local/state/lo0/migrations.json"
	launcher    = "/home/u/.local/bin/claude-local"
	variant     = "lo0/qwen3-coder-30b-a3b:64k"
)

func installedService(t *testing.T) (*app.Service, app.Assessment) {
	t.Helper()
	files := migratetest.NewFiles(map[string]string{
		journalPath: `{"changes":[
			{"id":1,"at":"2026-11-05T07:55:00Z","runtime":"ollama","runtimeName":"Ollama","baseUrl":"http://127.0.0.1:11434","model":"` + variant + `"},
			{"id":2,"at":"2026-11-05T07:55:00Z","runtime":"ollama","runtimeName":"Ollama","baseUrl":"http://127.0.0.1:11434","model":"` + variant + `"},
			{"id":3,"at":"2026-11-05T07:56:00Z","path":"` + launcher + `"}]}`,
		launcher: "export ANTHROPIC_MODEL='" + variant + "'\n",
	})
	mig := migrator(files)
	mig.Effects.Journal = migrate.Journal{Path: journalPath}
	res := result()
	res.Machine.Disk.FreeBytes = 100 << 30
	res.Runtimes = []runtimes.Runtime{
		{ID: runtimes.Ollama, Name: "Ollama", Installed: true, Running: true, Version: "0.34.4", Models: []runtimes.Model{
			{Name: "mystery:2b", SizeBytes: 2 << 30},
			{Name: variant, SizeBytes: 18 << 30, Parent: "qwen3-coder:30b", Quantization: "Q4_K_M"},
			{Name: "qwen3-coder:30b", SizeBytes: 18 << 30, Quantization: "Q4_K_M"},
		}},
		{ID: runtimes.LMStudio, Name: "LM Studio", Installed: true, Models: []runtimes.Model{{Name: "some-model", SizeBytes: 5 << 30}}},
	}
	return assess(t, res, mig)
}

func TestInventory(t *testing.T) {
	svc, a := installedService(t)
	inv, err := svc.Inventory(a)
	if err != nil {
		t.Fatal(err)
	}
	if inv.DiskBytes != 25<<30 || inv.FreeBytes != 100<<30 || len(inv.Changes) != 3 {
		t.Errorf("disk %d, free %d, %d changes; want shared weights counted once", inv.DiskBytes, inv.FreeBytes, len(inv.Changes))
	}
	ollama := inv.Runtimes[0]
	names := make([]string, 0, len(ollama.Models))
	for _, m := range ollama.Models {
		names = append(names, m.Name)
	}
	if want := []string{"mystery:2b", "qwen3-coder:30b", variant}; !slices.Equal(names, want) || !ollama.Removable {
		t.Fatalf("models %v, removable %v; want each model followed by those made from it", names, ollama.Removable)
	}
	base, v := ollama.Models[1], ollama.Models[2]
	if base.CatalogName != "Qwen3-Coder-30B-A3B" || base.Bytes != 18<<30 || !base.MadeByLo0.IsZero() {
		t.Errorf("base = %+v", base)
	}
	if v.Parent != "qwen3-coder:30b" || v.Bytes != 0 || v.MadeByLo0.IsZero() || v.CatalogName != "Qwen3-Coder-30B-A3B" ||
		!slices.Equal(v.UsedBy, []string{launcher}) {
		t.Errorf("variant = %+v; want it to share the weights, be lo0's and be used by the launcher", v)
	}
	if inv.Runtimes[1].Removable {
		t.Error("LM Studio models are marked removable")
	}
}

func TestPlanRemove(t *testing.T) {
	svc, a := installedService(t)
	for _, tt := range []struct {
		model string
		want  []string
	}{
		{"mystery:2b", []string{"ollama rm mystery:2b", "Frees 2 GiB."}},
		{"qwen3-coder:30b", []string{"ollama rm " + variant, "Made from qwen3-coder:30b", "ollama rm qwen3-coder:30b", "Frees 18 GiB."}},
		{variant, []string{"Frees no space: its weights are qwen3-coder:30b's", launcher + " uses it"}},
	} {
		p, err := svc.PlanRemove(a, runtimes.Ollama, tt.model)
		if err != nil {
			t.Fatal(err)
		}
		var lines []string
		for _, st := range p.Steps {
			lines = append(lines, st.Detail...)
		}
		detail := strings.Join(lines, "\n")
		for _, want := range tt.want {
			if !strings.Contains(detail, want) {
				t.Errorf("removing %s: detail %q lacks %q", tt.model, detail, want)
			}
		}
		if p.Command != "remove" || !p.Changes() {
			t.Errorf("removing %s: plan %+v", tt.model, p)
		}
	}
	if _, err := svc.PlanRemove(a, runtimes.LMStudio, "some-model"); err == nil || !strings.Contains(err.Error(), "remove some-model there") {
		t.Errorf("LM Studio removal: err %v", err)
	}
	if _, err := svc.PlanRemove(a, runtimes.Ollama, "nothing"); err == nil {
		t.Error("removing a model that is not there planned something")
	}
}

func TestPlanPullPreparesWithoutAHarness(t *testing.T) {
	svc, a := installedService(t)
	p, err := svc.PlanPull(a, app.MigrationChoice{Model: "qwen3-coder-30b-a3b"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Command != "pull" || p.Session != nil || p.HarnessName != "" || len(p.Steps) == 0 {
		t.Errorf("plan = %+v", p)
	}
	for _, s := range p.Steps {
		if s.Kind == migrate.KindWrite || s.Kind == migrate.KindCheck {
			t.Errorf("a download plan has step %q", s.Title)
		}
	}
	if !strings.Contains(p.Then[0], "lo0 try --model qwen3-coder-30b-a3b") {
		t.Errorf("then = %q", p.Then)
	}
}

var _ app.ModelRemover = backend.Ollama{}

func TestHarnessesAndInstall(t *testing.T) {
	svc, a := installedService(t)
	views := svc.Harnesses(t.Context(), a)
	if len(views) != 2 || views[0].ID != "claude-code" || !views[0].Recommended || !views[0].Checked || views[0].Fit.ID != "claude-code" {
		t.Fatalf("views = %+v", views)
	}
	if views[0].Installation.Installed || views[1].Installation.Installed {
		t.Errorf("harnesses not on PATH reported installed: %+v", views)
	}
	p, err := svc.PlanInstall(a, "opencode")
	if err != nil || p.Command != "install" || len(p.Steps) != 1 || !p.Changes() {
		t.Errorf("install plan = %+v, err %v", p, err)
	}
	if _, err := svc.PlanInstall(a, "aider"); err == nil {
		t.Error("planned installing a harness lo0 does not know")
	}
}
