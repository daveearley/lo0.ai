package migrate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrBusy means another lo0 holds the migration lock, so this one changed
// nothing.
var ErrBusy = errors.New("another lo0 is changing this machine; try again once it has finished")

// LockFile takes an exclusive lock on path, creating it and its directory
// if needed, and returns the function that releases it. It never waits: a
// lock another process holds is ErrBusy. The operating system releases the
// lock when the process exits, so a crash never leaves it held.
func LockFile(path string) (unlock func() error, err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	if err := lockFile(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() error {
		unlockErr := unlockFile(f)
		return errors.Join(unlockErr, f.Close())
	}, nil
}

// lockPath is the lock file that guards j.
func (j Journal) lockPath() string {
	return j.Path + ".lock"
}
