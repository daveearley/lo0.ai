package migrate

import (
	"context"
	"io/fs"
	"slices"
	"sync"
)

// Settle marks each write step in p done when running it now would change
// no file, so planning a migration that is already set up shows nothing
// left to do. It dry-runs the steps on read-only files and changes nothing;
// a step that fails stays pending. Without fx.Files, p is returned as is.
func Settle(p Plan, fx Effects) Plan {
	if fx.Files == nil {
		return p
	}
	// Planning only reads local files, as PlanUndo does, so there is
	// nothing for a caller to cancel.
	ctx := context.Background()
	steps := slices.Clone(p.Steps)
	for i, s := range steps {
		if s.Done || s.Kind != KindWrite || s.Run == nil {
			continue
		}
		dry := &readOnly{Files: fx.Files}
		view := Effects{Files: dry, Open: fx.Open, Now: fx.Now}
		if s.Run(ctx, view, func(int64, int64) {}) == nil && !dry.wrote() {
			steps[i].Done = true
		}
	}
	p.Steps = steps
	return p
}

// readOnly reads through Files and records, without making, every write or
// removal asked of it.
type readOnly struct {
	Files

	mu      sync.Mutex
	changed bool
}

func (r *readOnly) WriteFile(string, []byte, fs.FileMode) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.changed = true
	return nil
}

func (r *readOnly) Remove(string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.changed = true
	return nil
}

func (r *readOnly) wrote() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.changed
}
