package migrate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"time"

	"lo0.ai/lo0/internal/runtimes"
)

// Journal is the file where Apply records every change it makes, as it
// makes it, so that Undo can reverse them even after an interrupted run.
// The zero value records nothing.
type Journal struct {
	Path string
}

// Change is one thing Apply did. A file change has Path; a runtime change
// has Runtime and Model.
type Change struct {
	ID int       `json:"id"`
	At time.Time `json:"at"`

	Path string `json:"path,omitzero"`
	// Backup is the copy of the file Apply replaced. Empty means Apply
	// created the file.
	Backup string      `json:"backup,omitzero"`
	Perm   fs.FileMode `json:"perm,omitzero"`
	// SHA256 is the hash of what Apply wrote, so Undo can tell whether the
	// user has changed the file since.
	SHA256 string `json:"sha256,omitzero"`

	Runtime     runtimes.ID `json:"runtime,omitzero"`
	RuntimeName string      `json:"runtimeName,omitzero"`
	BaseURL     string      `json:"baseUrl,omitzero"`
	// Model is the runtime's name for a model Apply added.
	Model string `json:"model,omitzero"`
	// Bytes is the model's size on disk, when Apply downloaded it.
	Bytes int64 `json:"bytes,omitzero"`
	// Undo says what reversing a runtime change does, for the plan. The
	// default is removing Model from the runtime.
	Undo string `json:"undo,omitzero"`
}

type journalFile struct {
	Changes []Change `json:"changes"`
}

// Load returns the recorded changes, oldest first. A missing journal has
// none.
func (j Journal) Load(files Files) ([]Change, error) {
	if j.Path == "" {
		return nil, nil
	}
	data, err := files.ReadFile(j.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", j.Path, err)
	}
	var f journalFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("reading %s: %w", j.Path, err)
	}
	return f.Changes, nil
}

// add appends c with the next ID.
func (j Journal) add(files Files, c Change) error {
	if j.Path == "" {
		return nil
	}
	changes, err := j.Load(files)
	if err != nil {
		return err
	}
	for _, have := range changes {
		c.ID = max(c.ID, have.ID)
	}
	c.ID++
	return j.save(files, append(changes, c))
}

// forget removes the change with id, once Undo has reversed it. The
// journal is removed when nothing is left in it.
func (j Journal) forget(files Files, id int) error {
	changes, err := j.Load(files)
	if err != nil {
		return err
	}
	changes = slices.DeleteFunc(changes, func(c Change) bool { return c.ID == id })
	if len(changes) == 0 {
		if err := files.Remove(j.Path); err != nil {
			return err
		}
		// The lock is held by this process while it forgets. Windows will
		// not remove an open file, and then the directory stays too.
		_ = files.Remove(j.lockPath())
		// The directory was made for the journal; it goes too unless
		// something else is in it, in which case removing it fails and
		// that is fine.
		_ = files.Remove(filepath.Dir(j.Path))
		return nil
	}
	return j.save(files, changes)
}

func (j Journal) save(files Files, changes []Change) error {
	data, err := json.MarshalIndent(journalFile{Changes: changes}, "", "  ")
	if err != nil {
		return err
	}
	if err := files.WriteFile(j.Path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("recording the change in %s: %w", j.Path, err)
	}
	return nil
}

// Record adds c to the journal with the time it happened. Actions call it
// before a change they can detect afterwards, or right after one they
// cannot, so the journal is never behind what was done.
func (fx Effects) Record(c Change) error {
	if fx.Journal.Path == "" {
		return nil
	}
	c.At = fx.Now()
	return fx.Journal.add(fx.Files, c)
}

func hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Forget drops the journal's record of adding model to runtime rt, once the
// model has been removed some other way, so Undo does not try again.
func (fx Effects) Forget(rt runtimes.ID, model string) error {
	if fx.Journal.Path == "" {
		return nil
	}
	changes, err := fx.Journal.Load(fx.Files)
	if err != nil {
		return err
	}
	for _, c := range changes {
		if c.Runtime == rt && c.Model == model {
			if err := fx.Journal.forget(fx.Files, c.ID); err != nil {
				return err
			}
		}
	}
	return nil
}
