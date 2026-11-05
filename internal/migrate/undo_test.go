package migrate_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/migrate/migratetest"
	"lo0.ai/lo0/internal/runtimes"
)

const journalPath = "/home/u/.local/state/lo0/migrations.json"

func journaled(files *migratetest.Files) migrate.Effects {
	fx := effects(files)
	fx.Journal = migrate.Journal{Path: journalPath}
	return fx
}

func write(path, content string) migrate.Step {
	return migrate.Step{Kind: migrate.KindWrite, Title: "write " + path, Run: migrate.WriteFile(path, 0o644, func([]byte) ([]byte, error) {
		return []byte(content), nil
	})}
}

// recordingBackend reverts runtime changes by remembering them.
type recordingBackend struct{ reverted *[]string }

func (recordingBackend) ID() runtimes.ID { return runtimes.Ollama }
func (recordingBackend) Prepare(migrate.Request, migrate.Protocol) (migrate.Served, []migrate.Step, error) {
	return migrate.Served{}, nil, nil
}

func (b recordingBackend) Revert(_ context.Context, _ migrate.Effects, c migrate.Change) error {
	*b.reverted = append(*b.reverted, c.Model)
	return nil
}

func pull(model string) migrate.Step {
	return migrate.Step{Kind: migrate.KindDownload, Title: "pull " + model, Run: func(_ context.Context, fx migrate.Effects, _ func(int64, int64)) error {
		return fx.Record(migrate.Change{Runtime: runtimes.Ollama, RuntimeName: "Ollama", Model: model, Bytes: 100})
	}}
}

func titles(p migrate.Plan) []string {
	out := make([]string, len(p.Steps))
	for i, s := range p.Steps {
		out[i] = s.Title
	}
	return out
}

func TestUndoReversesEveryChangeNewestFirst(t *testing.T) {
	files := migratetest.NewFiles(map[string]string{"/home/u/.config/opencode/opencode.json": "mine"})
	fx := journaled(files)
	plan := migrate.Plan{Steps: []migrate.Step{
		pull("hf.co/x:Q4_K_M"),
		write("/home/u/.local/bin/claude-local", "launcher"),
		write("/home/u/.config/opencode/opencode.json", "lo0's"),
	}}
	if _, err := migrate.Apply(t.Context(), plan, fx, nil); err != nil {
		t.Fatal(err)
	}
	changes, err := fx.Journal.Load(files)
	if err != nil || len(changes) != 3 || changes[0].Model != "hf.co/x:Q4_K_M" || changes[2].Backup == "" || changes[1].Backup != "" {
		t.Fatalf("journal = %+v, %v", changes, err)
	}

	var reverted []string
	undo, err := migrate.PlanUndo(fx.Journal, files, []migrate.Backend{recordingBackend{&reverted}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"Restore opencode.json from before lo0",
		"Delete claude-local",
		"Remove hf.co/x:Q4_K_M from Ollama",
	}
	if !undo.Undo || !slices.Equal(titles(undo), want) {
		t.Fatalf("undo steps = %q, want %q", titles(undo), want)
	}
	if _, err := migrate.Apply(t.Context(), undo, fx, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := files.Get("/home/u/.config/opencode/opencode.json"); got != "mine" {
		t.Errorf("opencode.json = %q, want the user's version back", got)
	}
	if names := files.Names(); !slices.Equal(names, []string{"/home/u/.config/opencode/opencode.json"}) {
		t.Errorf("files left = %v, want only the restored one: no launcher, backup or journal", names)
	}
	if !slices.Contains(files.Removed(), "/home/u/.local/state/lo0") {
		t.Errorf("the journal's directory was not removed: %v", files.Removed())
	}
	if !slices.Equal(reverted, []string{"hf.co/x:Q4_K_M"}) {
		t.Errorf("reverted = %v", reverted)
	}
	again, _ := migrate.PlanUndo(fx.Journal, files, []migrate.Backend{recordingBackend{&reverted}})
	if len(again.Steps) != 0 {
		t.Errorf("a second undo has steps: %q", titles(again))
	}
}

func TestUndoLeavesFilesTheUserChanged(t *testing.T) {
	files := migratetest.NewFiles(nil)
	fx := journaled(files)
	if _, err := migrate.Apply(t.Context(), migrate.Plan{Steps: []migrate.Step{write("/home/u/a", "lo0's")}}, fx, nil); err != nil {
		t.Fatal(err)
	}
	if err := files.WriteFile("/home/u/a", []byte("edited by the user"), 0o644); err != nil {
		t.Fatal(err)
	}
	undo, err := migrate.PlanUndo(fx.Journal, files, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(undo.Steps) != 1 || !strings.Contains(undo.Steps[0].Title, "changed after lo0 wrote it") {
		t.Fatalf("steps = %q", titles(undo))
	}
	if _, err := migrate.Apply(t.Context(), undo, fx, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := files.Get("/home/u/a"); got != "edited by the user" {
		t.Errorf("file = %q, want the user's edit kept", got)
	}
	if _, ok := files.Get(journalPath); ok {
		t.Error("the change is still in the journal")
	}
}

// The plan can sit on screen while the user keeps working. An edit made
// then is kept, and the change stays in the journal for the next undo.
func TestUndoKeepsAnEditMadeAfterPlanning(t *testing.T) {
	for name, original := range map[string]string{"delete": "", "restore": "the user's original"} {
		t.Run(name, func(t *testing.T) {
			var seed map[string]string
			if original != "" {
				seed = map[string]string{"/home/u/a": original}
			}
			files := migratetest.NewFiles(seed)
			fx := journaled(files)
			if _, err := migrate.Apply(t.Context(), migrate.Plan{Steps: []migrate.Step{write("/home/u/a", "lo0's")}}, fx, nil); err != nil {
				t.Fatal(err)
			}
			undo, err := migrate.PlanUndo(fx.Journal, files, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := files.WriteFile("/home/u/a", []byte("edited while the plan was shown"), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := migrate.Apply(t.Context(), undo, fx, nil); !errors.Is(err, migrate.ErrChangedSincePlan) {
				t.Fatalf("Apply = %v, want ErrChangedSincePlan", err)
			}
			if got, _ := files.Get("/home/u/a"); got != "edited while the plan was shown" {
				t.Errorf("file = %q, want the edit kept", got)
			}
			if changes, _ := fx.Journal.Load(files); len(changes) != 1 {
				t.Errorf("journal = %+v, want the change kept for the next undo", changes)
			}
		})
	}
}

func TestUndoFollowsAFileWrittenTwice(t *testing.T) {
	files := migratetest.NewFiles(nil)
	fx := journaled(files)
	if _, err := migrate.Apply(t.Context(), migrate.Plan{Steps: []migrate.Step{write("/home/u/a", "first")}}, fx, nil); err != nil {
		t.Fatal(err)
	}
	fx.Now = func() time.Time { return migratetest.Clock().Add(time.Hour) }
	if _, err := migrate.Apply(t.Context(), migrate.Plan{Steps: []migrate.Step{write("/home/u/a", "second")}}, fx, nil); err != nil {
		t.Fatal(err)
	}
	undo, err := migrate.PlanUndo(fx.Journal, files, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Restore a from before lo0", "Delete a"}; !slices.Equal(titles(undo), want) {
		t.Fatalf("steps = %q, want %q", titles(undo), want)
	}
	if _, err := migrate.Apply(t.Context(), undo, fx, nil); err != nil {
		t.Fatal(err)
	}
	if names := files.Names(); len(names) != 0 {
		t.Errorf("files left = %v", names)
	}
}

func TestUndoResumesAfterAFailure(t *testing.T) {
	files := migratetest.NewFiles(nil)
	fx := journaled(files)
	plan := migrate.Plan{Steps: []migrate.Step{pull("m"), write("/home/u/a", "x")}}
	if _, err := migrate.Apply(t.Context(), plan, fx, nil); err != nil {
		t.Fatal(err)
	}
	down := failingBackend{}
	undo, _ := migrate.PlanUndo(fx.Journal, files, []migrate.Backend{down})
	if _, err := migrate.Apply(t.Context(), undo, fx, nil); err == nil {
		t.Fatal("undo succeeded although the runtime is down")
	}
	left, _ := migrate.PlanUndo(fx.Journal, files, []migrate.Backend{down})
	if want := []string{"Remove m from Ollama"}; !slices.Equal(titles(left), want) {
		t.Errorf("after the failure, undo has %q left, want %q", titles(left), want)
	}
}

type failingBackend struct{}

func (failingBackend) ID() runtimes.ID { return runtimes.Ollama }
func (failingBackend) Prepare(migrate.Request, migrate.Protocol) (migrate.Served, []migrate.Step, error) {
	return migrate.Served{}, nil, nil
}

func (failingBackend) Revert(context.Context, migrate.Effects, migrate.Change) error {
	return errors.New("Ollama is not running")
}

func TestUndoNeedsTheBackendThatMadeTheChange(t *testing.T) {
	files := migratetest.NewFiles(nil)
	fx := journaled(files)
	if _, err := migrate.Apply(t.Context(), migrate.Plan{Steps: []migrate.Step{pull("m")}}, fx, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := migrate.PlanUndo(fx.Journal, files, nil); err == nil {
		t.Error("an undo was planned for a runtime no backend handles")
	}
}

func TestWithoutAJournalNothingIsRecorded(t *testing.T) {
	files := migratetest.NewFiles(nil)
	if _, err := migrate.Apply(t.Context(), migrate.Plan{Steps: []migrate.Step{write("/home/u/a", "x")}}, effects(files), nil); err != nil {
		t.Fatal(err)
	}
	if names := files.Names(); !slices.Equal(names, []string{"/home/u/a"}) {
		t.Errorf("files = %v", names)
	}
}

func TestBackupsInTheSameSecondDoNotCollide(t *testing.T) {
	files := migratetest.NewFiles(map[string]string{"/home/u/a": "original"})
	fx := journaled(files)
	for _, content := range []string{"one", "two"} {
		if _, err := migrate.Apply(t.Context(), migrate.Plan{Steps: []migrate.Step{write("/home/u/a", content)}}, fx, nil); err != nil {
			t.Fatal(err)
		}
	}
	first, _ := files.Get("/home/u/a.lo0-backup-20261104-183000")
	second, _ := files.Get("/home/u/a.lo0-backup-20261104-183000-2")
	if first != "original" || second != "one" {
		t.Errorf("backups = %q and %q, want each earlier version kept", first, second)
	}
}

func TestForgetDropsAModelsRecords(t *testing.T) {
	const path = "/state/migrations.json"
	files := migratetest.NewFiles(map[string]string{path: `{"changes":[
		{"id":1,"runtime":"ollama","model":"lo0/a:64k"},
		{"id":2,"runtime":"ollama","model":"lo0/a:64k"},
		{"id":3,"runtime":"ollama","model":"lo0/b:64k"}]}`})
	fx := migrate.Effects{Files: files, Journal: migrate.Journal{Path: path}}
	if err := fx.Forget(runtimes.Ollama, "lo0/a:64k"); err != nil {
		t.Fatal(err)
	}
	left, err := fx.Journal.Load(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 || left[0].Model != "lo0/b:64k" {
		t.Errorf("journal = %+v, want only lo0/b", left)
	}
}

func TestUndoRevertsARepeatedRuntimeChangeOnce(t *testing.T) {
	files := migratetest.NewFiles(nil)
	fx := journaled(files)
	plan := migrate.Plan{Steps: []migrate.Step{pull("lo0/a:256k"), pull("lo0/b:256k"), pull("lo0/b:256k")}}
	if _, err := migrate.Apply(t.Context(), plan, fx, nil); err != nil {
		t.Fatal(err)
	}
	var reverted []string
	undo, err := migrate.PlanUndo(fx.Journal, files, []migrate.Backend{recordingBackend{&reverted}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Remove lo0/b:256k from Ollama", "Remove lo0/a:256k from Ollama"}
	if !slices.Equal(titles(undo), want) {
		t.Fatalf("undo steps = %q, want %q", titles(undo), want)
	}
	if _, err := migrate.Apply(t.Context(), undo, fx, nil); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(reverted, []string{"lo0/b:256k", "lo0/a:256k"}) {
		t.Errorf("reverted = %v", reverted)
	}
	if changes, _ := fx.Journal.Load(files); len(changes) != 0 {
		t.Errorf("journal still holds %+v", changes)
	}
}
