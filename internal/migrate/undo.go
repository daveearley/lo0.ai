package migrate

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
)

// PlanUndo returns the plan that reverses every change in the journal,
// newest first. It reads the files involved, without changing them, so a
// file the user has edited since lo0 wrote it is left as it is. Each step
// removes its change from the journal once done, so an interrupted undo
// picks up where it stopped.
func PlanUndo(j Journal, files Files, backends []Backend) (Plan, error) {
	changes, err := j.Load(files)
	if err != nil {
		return Plan{}, err
	}
	p := Plan{Undo: true}
	paths := map[string]content{}
	// A runtime change made twice, such as a model lo0 created again, is
	// undone once; that step forgets every record of it.
	repeats := map[Change][]int{}
	for _, c := range changes {
		if c.Path == "" {
			k := runtimeKey(c)
			repeats[k] = append(repeats[k], c.ID)
		}
	}
	for _, c := range slices.Backward(changes) {
		var s Step
		if c.Path != "" {
			s, err = undoFile(files, paths, c)
		} else {
			ids, planned := repeats[runtimeKey(c)]
			if !planned {
				continue
			}
			delete(repeats, runtimeKey(c))
			s, err = undoRuntime(backends, c, ids)
		}
		if err != nil {
			return Plan{}, err
		}
		p.Steps = append(p.Steps, s)
	}
	if len(p.Steps) > 0 {
		p.Then = []string{"Everything lo0 migrate changed is as it was before."}
	}
	return p, nil
}

// content is what a file will hold once the steps planned so far have run.
type content struct {
	missing bool
	sum     string
}

func read(files Files, path string) (content, error) {
	data, err := files.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return content{missing: true}, nil
	case err != nil:
		return content{}, fmt.Errorf("reading %s: %w", path, err)
	}
	return content{sum: hash(data)}, nil
}

// undoFile plans reversing one write. paths carries each file's content
// forward from the newer steps, so a file lo0 wrote twice is checked
// against what the newer undo leaves, not against what is on disk now.
func undoFile(files Files, paths map[string]content, c Change) (Step, error) {
	cur, seen := paths[c.Path]
	if !seen {
		var err error
		if cur, err = read(files, c.Path); err != nil {
			return Step{}, err
		}
	}
	// Titles name the file and the detail gives the full path, which front
	// ends shorten or redact.
	name := filepath.Base(c.Path)
	s := Step{Kind: KindWrite, Detail: []string{c.Path}}
	forget := func(_ context.Context, fx Effects, progress func(done, total int64)) error {
		progress(1, 1)
		return fx.Journal.forget(fx.Files, c.ID)
	}
	var backup content
	if c.Backup != "" {
		var err error
		if backup, err = read(files, c.Backup); err != nil {
			return Step{}, err
		}
	}
	planned := cur
	switch {
	case !cur.missing && cur.sum != c.SHA256:
		s.Title = "Leave " + name + " as it is: it changed after lo0 wrote it"
		if c.Backup != "" && !backup.missing {
			s.Detail = []string{c.Path, "The version from before lo0 is in " + c.Backup + "."}
		}
		s.Run = forget
	case c.Backup == "" && cur.missing:
		s.Title = name + " is already gone"
		s.Run = forget
	case c.Backup == "":
		s.Title = "Delete " + name
		s.Run = func(ctx context.Context, fx Effects, progress func(done, total int64)) error {
			if err := unchanged(fx.Files, c.Path, planned); err != nil {
				return err
			}
			if err := fx.Files.Remove(c.Path); err != nil {
				return fmt.Errorf("deleting %s: %w", c.Path, err)
			}
			return forget(ctx, fx, progress)
		}
		cur = content{missing: true}
	case backup.missing:
		s.Title = "Leave " + name + " as it is: its backup is gone"
		s.Detail = []string{c.Path, "lo0 kept the earlier version in " + c.Backup + ", which is no longer there."}
		s.Run = forget
	default:
		s.Title = "Restore " + name + " from before lo0"
		s.Detail = []string{c.Path, c.Backup}
		s.Run = func(ctx context.Context, fx Effects, progress func(done, total int64)) error {
			if err := unchanged(fx.Files, c.Path, planned); err != nil {
				return err
			}
			old, err := fx.Files.ReadFile(c.Backup)
			if err != nil {
				return fmt.Errorf("reading the backup %s: %w", c.Backup, err)
			}
			if err := fx.Files.WriteFile(c.Path, old, c.Perm); err != nil {
				return fmt.Errorf("restoring %s: %w", c.Path, err)
			}
			if err := fx.Files.Remove(c.Backup); err != nil {
				return fmt.Errorf("deleting the backup %s: %w", c.Backup, err)
			}
			return forget(ctx, fx, progress)
		}
		cur = backup
	}
	paths[c.Path] = cur
	return s, nil
}

// ErrChangedSincePlan means a file changed between planning an undo and
// running it, so the step left it alone.
var ErrChangedSincePlan = errors.New("it changed after the undo was planned, so lo0 left it as it is; run lo0 migrate --undo again to see the new plan")

// unchanged checks, as a step runs, that path still holds what the plan
// saw. The plan can sit on screen while the user keeps working, and an edit
// made then must not be deleted or overwritten. The journal keeps the
// change, so the next undo plans it again.
func unchanged(files Files, path string, want content) error {
	now, err := read(files, path)
	if err != nil {
		return err
	}
	if now != want {
		return fmt.Errorf("%s: %w", path, ErrChangedSincePlan)
	}
	return nil
}

// runtimeKey is what makes two runtime changes the same change: the same
// thing done to the same runtime, whenever it was recorded.
func runtimeKey(c Change) Change {
	return Change{Runtime: c.Runtime, BaseURL: c.BaseURL, Model: c.Model, Undo: c.Undo}
}

// undoRuntime plans reversing c, forgetting every journal entry in ids,
// which records the same change.
func undoRuntime(backends []Backend, c Change, ids []int) (Step, error) {
	i := slices.IndexFunc(backends, func(b Backend) bool { return b.ID() == c.Runtime })
	if i < 0 {
		return Step{}, fmt.Errorf("lo0 cannot undo changes to %q", c.Runtime)
	}
	b := backends[i]
	return Step{
		Kind:  KindRuntime,
		Title: cmp.Or(c.Undo, "Remove "+c.Model+" from "+c.RuntimeName),
		Bytes: c.Bytes,
		Run: func(ctx context.Context, fx Effects, progress func(done, total int64)) error {
			if err := b.Revert(ctx, fx, c); err != nil {
				return err
			}
			progress(1, 1)
			for _, id := range ids {
				if err := fx.Journal.forget(fx.Files, id); err != nil {
					return err
				}
			}
			return nil
		},
	}, nil
}
