package migrate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"lo0.ai/lo0/internal/system"
)

// Files reads and writes files by absolute path.
type Files interface {
	ReadFile(name string) ([]byte, error)
	// WriteFile creates name and its parent directories, or replaces it.
	// perm applies to a new file; a file that exists keeps its mode, and a
	// symlink keeps pointing where it did. A private perm, one that gives
	// group and others nothing, marks contents such as secrets: a file that
	// exists loses its group and other bits, and a symlink into a git work
	// tree the link is not in is refused with ErrPrivateInRepo.
	WriteFile(name string, data []byte, perm fs.FileMode) error
	// Remove deletes name, which is a file or an empty directory. One that
	// is already gone is not an error.
	Remove(name string) error
}

// Effects are the only ways Apply changes anything. Every field but
// Journal is required.
type Effects struct {
	Files Files
	// Open returns a read-only view of a directory, for copying trees.
	Open   func(dir string) fs.FS
	Runner system.Runner
	// Client talks to local runtimes. It must not time out on its own:
	// downloads take as long as they take, and Apply bounds what needs it.
	Client *http.Client
	Now    func() time.Time
	// Journal records each change for Undo.
	Journal Journal
	// Lock, when set, takes an exclusive lock on a file for as long as
	// Apply runs, so two lo0 processes never interleave their journal
	// writes and lose each other's changes. LockFile is the real one.
	Lock func(path string) (unlock func() error, err error)
}

// ErrPrivateInRepo means a private write would follow a symlink into a git
// repository, such as a dotfiles repository, where the secrets it holds
// could be committed.
var ErrPrivateInRepo = errors.New("it links into a git repository, and lo0 would write secrets there; replace the link with a copy of the file, or move the secrets out of that repository's reach, and run lo0 again")

// OSFiles is Files on the real file system. Writes go to a temporary file
// that replaces the target, so a failed write never leaves half a file.
type OSFiles struct{}

// ReadFile implements Files.
func (OSFiles) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

// WriteFile implements Files.
func (OSFiles) WriteFile(name string, data []byte, perm fs.FileMode) error {
	private := perm&0o077 == 0
	if target, err := filepath.EvalSymlinks(name); err == nil {
		if private && target != name {
			if repo := workTree(filepath.Dir(target)); repo != "" && repo != workTree(filepath.Dir(name)) {
				return fmt.Errorf("%s: %w", name, ErrPrivateInRepo)
			}
		}
		name = target
	}
	if info, err := os.Stat(name); err == nil {
		mode := info.Mode().Perm()
		if private {
			mode &^= 0o077
		}
		perm = mode
	}
	dir := filepath.Dir(name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(name)+".lo0-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing %s: %w", name, err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), name)
}

// workTree is the git work tree dir is in, or empty when it is in none.
func workTree(dir string) string {
	for {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// Remove implements Files.
func (OSFiles) Remove(name string) error {
	if err := os.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// BackupSuffix marks the copies lo0 keeps of files it replaces.
const BackupSuffix = ".lo0-backup"

// WriteFile returns an action that writes path. build receives the current
// contents, or nil when there is no file, and returns the new contents. A
// file whose contents would change is first copied to
// <path>.lo0-backup-<time>, readable only by the user, and the write is
// recorded in the journal before it happens, so every change can be undone
// even if lo0 stops halfway; writing the same contents again does nothing.
func WriteFile(path string, perm fs.FileMode, build func(old []byte) ([]byte, error)) Action {
	return func(_ context.Context, fx Effects, progress func(done, total int64)) error {
		old, err := fx.Files.ReadFile(path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			old = nil
		case err != nil:
			return fmt.Errorf("reading %s: %w", path, err)
		}
		data, err := build(old)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if old != nil && bytes.Equal(old, data) {
			progress(1, 1)
			return nil
		}
		change := Change{Path: path, Perm: perm, SHA256: hash(data)}
		if old != nil {
			change.Backup = backupName(fx, path)
			if err := fx.Files.WriteFile(change.Backup, old, 0o600); err != nil {
				return fmt.Errorf("backing up %s: %w", path, err)
			}
		}
		// Undo treats a recorded write that never happened as a file that
		// changed since, so recording first loses nothing.
		if err := fx.Record(change); err != nil {
			return err
		}
		if err := fx.Files.WriteFile(path, data, perm); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
		progress(1, 1)
		return nil
	}
}

// backupName is a new name for a copy of path, so two writes in the same
// second never share a backup.
func backupName(fx Effects, path string) string {
	base := path + BackupSuffix + "-" + fx.Now().Format("20060102-150405")
	name := base
	for n := 2; ; n++ {
		if _, err := fx.Files.ReadFile(name); errors.Is(err, fs.ErrNotExist) {
			return name
		}
		name = base + "-" + strconv.Itoa(n)
	}
}
