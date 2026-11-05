package migrate_test

import (
	"context"
	"errors"
	"maps"
	"path/filepath"
	"testing"

	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/migrate/migratetest"
)

func TestSettleMarksUnchangedWritesDone(t *testing.T) {
	write := func(name, data string) migrate.Step {
		return step(migrate.KindWrite, name, migrate.WriteFile(name, 0o644, func([]byte) ([]byte, error) { return []byte(data), nil }))
	}
	failing := step(migrate.KindWrite, "failing", func(context.Context, migrate.Effects, func(int64, int64)) error {
		return errors.New("cannot")
	})
	tests := []struct {
		name string
		step migrate.Step
		done bool
	}{
		{"file already as the plan writes it", write("/home/u/same", "launcher"), true},
		{"file with other contents", write("/home/u/other", "launcher"), false},
		{"file not there yet", write("/home/u/missing", "launcher"), false},
		{"write that fails", failing, false},
		{"download, which settle leaves alone", step(migrate.KindDownload, "pull", ok), false},
		{"check, which settle leaves alone", step(migrate.KindCheck, "check", ok), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := migratetest.NewFiles(map[string]string{"/home/u/same": "launcher", "/home/u/other": "old"})
			fx := effects(files)
			fx.Journal = migrate.Journal{Path: filepath.Join(t.TempDir(), "migrations.json")}
			before := contents(files)

			p := migrate.Settle(migrate.Plan{Steps: []migrate.Step{tt.step}}, fx)

			if got := p.Steps[0].Done; got != tt.done {
				t.Errorf("done = %v, want %v", got, tt.done)
			}
			if after := contents(files); !maps.Equal(before, after) || len(files.Removed()) > 0 {
				t.Errorf("settle changed files: before %v, after %v, removed %v", before, after, files.Removed())
			}
			if changes, err := fx.Journal.Load(files); err != nil || len(changes) > 0 {
				t.Errorf("settle recorded changes %v (%v)", changes, err)
			}
		})
	}
}

func TestSettleLeavesThePlanItWasGiven(t *testing.T) {
	files := migratetest.NewFiles(map[string]string{"/home/u/same": "x"})
	p := migrate.Plan{Steps: []migrate.Step{
		step(migrate.KindWrite, "same", migrate.WriteFile("/home/u/same", 0o644, func([]byte) ([]byte, error) { return []byte("x"), nil })),
	}}
	settled := migrate.Settle(p, effects(files))
	if p.Steps[0].Done || !settled.Steps[0].Done {
		t.Errorf("settle changed the plan it was given or did not mark the copy: %v, %v", p.Steps[0].Done, settled.Steps[0].Done)
	}
	if got := migrate.Settle(p, migrate.Effects{}); got.Steps[0].Done {
		t.Errorf("without files, settle marked a step done")
	}
}

func TestPendingIgnoresChecks(t *testing.T) {
	done := step(migrate.KindWrite, "write", ok)
	done.Done = true
	tests := []struct {
		name  string
		steps []migrate.Step
		want  bool
	}{
		{"only checks left", []migrate.Step{done, step(migrate.KindCheck, "check", ok)}, false},
		{"a write left", []migrate.Step{step(migrate.KindWrite, "write", ok), step(migrate.KindCheck, "check", ok)}, true},
		{"a step for the user left", []migrate.Step{done, {Kind: migrate.KindManual, Title: "Start Ollama"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (migrate.Plan{Steps: tt.steps}).Pending(); got != tt.want {
				t.Errorf("Pending() = %v, want %v", got, tt.want)
			}
		})
	}
}

// contents is every file in files with what it holds.
func contents(files *migratetest.Files) map[string]string {
	out := map[string]string{}
	for _, name := range files.Names() {
		out[name], _ = files.Get(name)
	}
	return out
}
