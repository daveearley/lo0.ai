package jsonl_test

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"lo0.ai/lo0/internal/jsonl"
)

func TestRegular(t *testing.T) {
	fsys := fstest.MapFS{
		"a.jsonl":    {Data: []byte("{}\n")},
		"pipe.jsonl": {Mode: fs.ModeNamedPipe},
		"dev.jsonl":  {Mode: fs.ModeDevice | fs.ModeCharDevice},
		"link.jsonl": {Mode: fs.ModeSymlink, Data: []byte("a.jsonl")},
		"bad.jsonl":  {Mode: fs.ModeSymlink, Data: []byte("pipe.jsonl")},
	}
	want := map[string]bool{"a.jsonl": true, "link.jsonl": true}
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range entries {
		if got := jsonl.Regular(fsys, d.Name(), d); got != want[d.Name()] {
			t.Errorf("Regular(%s) = %v, want %v", d.Name(), got, want[d.Name()])
		}
	}
}
