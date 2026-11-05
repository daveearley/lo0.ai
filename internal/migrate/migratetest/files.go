// Package migratetest provides fakes for testing migrations without touching
// the real file system.
package migratetest

import (
	"fmt"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing/fstest"
	"time"
)

// Files is an in-memory migrate.Files keyed by absolute path. Paths are
// compared in slash form, so a test can name a file "/home/u/a" and still
// find what production code wrote as filepath.Join("/home/u", "a") on
// Windows.
type Files struct {
	mu      sync.Mutex
	files   map[string]File
	removed []string
	// Fail makes writes to the paths it lists fail.
	Fail map[string]error
}

// File is one file written to Files.
type File struct {
	Data []byte
	Perm fs.FileMode
}

// NewFiles returns Files holding the given contents.
func NewFiles(contents map[string]string) *Files {
	f := &Files{files: map[string]File{}}
	for name, data := range contents {
		f.files[key(name)] = File{Data: []byte(data), Perm: 0o644}
	}
	return f
}

// ReadFile implements migrate.Files.
func (f *Files) ReadFile(name string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	file, ok := f.files[key(name)]
	if !ok {
		return nil, fmt.Errorf("open %s: %w", name, fs.ErrNotExist)
	}
	return slices.Clone(file.Data), nil
}

// WriteFile implements migrate.Files. Like the real one, a file that exists
// keeps its mode.
func (f *Files) WriteFile(name string, data []byte, perm fs.FileMode) error {
	for failing, err := range f.Fail {
		if key(failing) == key(name) {
			return err
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if have, ok := f.files[key(name)]; ok {
		perm = have.Perm
	}
	f.files[key(name)] = File{Data: slices.Clone(data), Perm: perm}
	return nil
}

// Remove implements migrate.Files.
func (f *Files) Remove(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.files, key(name))
	f.removed = append(f.removed, key(name))
	return nil
}

// Removed lists every path passed to Remove, in order.
func (f *Files) Removed() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.removed)
}

// Get returns a file's contents and whether it exists.
func (f *Files) Get(name string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	file, ok := f.files[key(name)]
	return string(file.Data), ok
}

// Perm returns the permissions a file was written with.
func (f *Files) Perm(name string) fs.FileMode {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.files[key(name)].Perm
}

// Names lists every path in slash form, sorted.
func (f *Files) Names() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Sorted(maps.Keys(f.files))
}

// Open returns a read-only view of the files under dir, for Effects.Open.
func (f *Files) Open(dir string) fs.FS {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := fstest.MapFS{}
	prefix := strings.TrimSuffix(key(filepath.Clean(dir)), "/") + "/"
	for name, file := range f.files {
		if rel, ok := strings.CutPrefix(name, prefix); ok {
			out[rel] = &fstest.MapFile{Data: slices.Clone(file.Data), Mode: file.Perm}
		}
	}
	return out
}

func key(name string) string {
	return filepath.ToSlash(name)
}

// Clock returns a fixed time for Effects.Now.
func Clock() time.Time {
	return time.Date(2026, 11, 4, 18, 30, 0, 0, time.UTC)
}
