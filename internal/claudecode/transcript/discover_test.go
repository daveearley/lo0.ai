package transcript_test

import (
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"

	"lo0.ai/lo0/internal/claudecode/transcript"
)

// unreadable is a file system in which one folder cannot be listed.
type unreadable struct {
	fstest.MapFS
	dir string
}

func (u unreadable) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == u.dir {
		return nil, fs.ErrPermission
	}
	return u.MapFS.ReadDir(name)
}

func TestDiscoverSkipsAFolderItCannotRead(t *testing.T) {
	fsys := unreadable{MapFS: fstest.MapFS{
		"-home-u-a/s1.jsonl": {},
		"-home-u-b/s2.jsonl": {},
		"-home-u-c/s3.jsonl": {},
	}, dir: "-home-u-b"}
	sessions, err := transcript.Discover(fsys)
	if len(sessions) != 2 || !errors.Is(err, fs.ErrPermission) {
		t.Errorf("Discover = %d sessions, %v; want the two readable ones and the error", len(sessions), err)
	}
}
