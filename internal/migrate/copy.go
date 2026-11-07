package migrate

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
)

// CopyTree copies every file under src to dst, keeping files executable
// that were, each through WriteFile so undo can remove it.
func CopyTree(ctx context.Context, fx Effects, src, dst string) error {
	fsys := fx.Open(src)
	return fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("reading %s: %w", filepath.Join(src, filepath.FromSlash(p)), err)
		}
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return fmt.Errorf("reading %s: %w", filepath.Join(src, filepath.FromSlash(p)), err)
		}
		perm := fs.FileMode(0o644)
		if info, err := d.Info(); err == nil && info.Mode()&0o111 != 0 {
			perm = 0o755
		}
		write := WriteFile(filepath.Join(dst, filepath.FromSlash(path.Clean(p))), perm, func([]byte) ([]byte, error) { return data, nil })
		return write(ctx, fx, func(int64, int64) {})
	})
}
