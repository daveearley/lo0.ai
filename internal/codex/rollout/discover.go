package rollout

import (
	"errors"
	"io/fs"
	"path"
	"slices"
	"strings"

	"lo0.ai/lo0/internal/jsonl"
)

// Discover finds the rollout-<time>-<id>.jsonl session logs in fsys, which
// is rooted at a folder of them, nested in YYYY/MM/DD folders or flat.
// A folder that cannot be read is skipped and reported in the returned error
// with the rest; only an unreadable root stops the walk.
func Discover(fsys fs.FS) ([]string, error) {
	var out []string
	var errs []error
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			switch {
			case p == ".":
				return err
			case errors.Is(err, fs.ErrNotExist):
				return nil
			}
			errs = append(errs, err)
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !d.IsDir() && strings.HasPrefix(path.Base(p), "rollout-") && path.Ext(p) == ".jsonl" && jsonl.Regular(fsys, p, d) {
			out = append(out, p)
		}
		return nil
	})
	slices.Sort(out)
	return out, errors.Join(err, errors.Join(errs...))
}
