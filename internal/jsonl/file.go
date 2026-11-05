package jsonl

import "io/fs"

// Regular reports whether the entry d at p in fsys is a regular file, or a
// symlink to one. A device, FIFO or socket named like a log is not one: a
// FIFO blocks opening it and /dev/zero never ends, so a scan must not read
// either.
func Regular(fsys fs.FS, p string, d fs.DirEntry) bool {
	if d.Type()&fs.ModeSymlink == 0 {
		return d.Type().IsRegular()
	}
	info, err := fs.Stat(fsys, p)
	return err == nil && info.Mode().IsRegular()
}
