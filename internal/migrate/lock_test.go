package migrate_test

import (
	"errors"
	"path/filepath"
	"testing"

	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/migrate/migratetest"
)

func TestLockFileIsExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "migrations.json.lock")
	unlock, err := migrate.LockFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := migrate.LockFile(path); !errors.Is(err, migrate.ErrBusy) {
		t.Errorf("second lock = %v, want ErrBusy", err)
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
	again, err := migrate.LockFile(path)
	if err != nil {
		t.Fatalf("lock after unlock: %v", err)
	}
	if err := again(); err != nil {
		t.Fatal(err)
	}
}

// Apply holds the lock while it runs, so a second lo0 changes nothing
// rather than writing over the first one's journal.
func TestApplyRefusesWhileAnotherHoldsTheLock(t *testing.T) {
	files := migratetest.NewFiles(nil)
	fx := journaled(files)
	fx.Lock = func(string) (func() error, error) { return nil, migrate.ErrBusy }
	if _, err := migrate.Apply(t.Context(), migrate.Plan{Steps: []migrate.Step{write("/home/u/a", "lo0's")}}, fx, nil); !errors.Is(err, migrate.ErrBusy) {
		t.Fatalf("Apply = %v, want ErrBusy", err)
	}
	if names := files.Names(); len(names) != 0 {
		t.Errorf("files written while locked: %v", names)
	}
}
