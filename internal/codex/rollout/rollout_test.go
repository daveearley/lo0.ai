package rollout_test

import (
	"encoding/json"
	"slices"
	"testing"
	"testing/fstest"

	"lo0.ai/lo0/internal/codex/rollout"
)

func decode(t *testing.T, in string) rollout.Payload {
	t.Helper()
	var p rollout.Payload
	if err := json.Unmarshal([]byte(in), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPatch(t *testing.T) {
	tests := []struct {
		name           string
		in             string
		files          []string
		added, removed int
	}{
		{
			"update and add",
			`{"input":"*** Begin Patch\n*** Update File: /a/x.go\n@@ func f\n-old\n+new\n context\n*** Add File: /a/y.md\n+one\n+two\n*** End Patch"}`,
			[]string{"/a/x.go", "/a/y.md"},
			3, 1,
		},
		{"delete", `{"input":"*** Begin Patch\n*** Delete File: /a/z.txt\n*** End Patch"}`, []string{"/a/z.txt"}, 0, 0},
		{"escaped path", `{"input":"*** Begin Patch\n*** Update File: C:\\src\\a.go\n+x\n*** End Patch"}`, []string{`C:\src\a.go`}, 1, 0},
		{"code, not a patch", `{"input":"const x = 1\n+y"}`, nil, 0, 0},
		{"object input", `{"input":{"x":1}}`, nil, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := decode(t, tt.in).Input
			if !slices.Equal(p.Files, tt.files) || p.Added != tt.added || p.Removed != tt.removed {
				t.Errorf("Patch = %+v, want files %q +%d -%d", p, tt.files, tt.added, tt.removed)
			}
		})
	}
}

func TestDiff(t *testing.T) {
	p := decode(t, `{"item":{"type":"FileChange","changes":{"/a.go":{"unified_diff":"--- a/a.go\n+++ b/a.go\n@@ -1,2 +1,2 @@\n-a\n-b\n+c\n d\n"}}}}`)
	d := p.Item.Changes["/a.go"].Diff
	if d != (rollout.Diff{Added: 1, Removed: 2}) {
		t.Errorf("Diff = %+v, want headers left out", d)
	}
}

func TestSourceAndFork(t *testing.T) {
	sub := decode(t, `{"source":{"subagent":{"thread_spawn":{"parent_thread_id":"p1","depth":1}}}}`)
	plain := decode(t, `{"source":"vscode","forked_from_id":"f1"}`)
	odd := decode(t, `{"source":{"subagent":"review"}}`)
	if sub.Source.Parent != "p1" || plain.Source.Parent != "" || plain.ForkedFrom != "f1" || odd.Source.Parent != "" {
		t.Errorf("sources = %+v, %+v, %+v", sub.Source, plain, odd.Source)
	}
}

func TestTaskError(t *testing.T) {
	tests := map[string]string{
		`{"error":{"message":"x","codex_error_info":"usage_limit_exceeded"}}`: "usage_limit_exceeded",
		`{"error":{"message":"stream disconnected"}}`:                         "unknown",
		`{"error":null}`: "",
		`{}`:             "",
	}
	for in, want := range tests {
		if got := decode(t, in).Error.Info; got != want {
			t.Errorf("%s: Info = %q, want %q", in, got, want)
		}
	}
}

func TestContentImages(t *testing.T) {
	p := decode(t, `{"content":[{"type":"input_text","text":"x"},{"type":"input_image"}],"output":"plain text output"}`)
	if p.Content.Images() != 1 || p.Output.Images() != 0 {
		t.Errorf("images = %d in content, %d in output", p.Content.Images(), p.Output.Images())
	}
}

func TestDiscover(t *testing.T) {
	fsys := fstest.MapFS{
		"2026/10/02/rollout-b.jsonl": {},
		"2026/10/01/rollout-a.jsonl": {},
		"2026/10/01/other.jsonl":     {},
		"2026/10/01/rollout-c.json":  {},
	}
	got, err := rollout.Discover(fsys)
	if err != nil || !slices.Equal(got, []string{"2026/10/01/rollout-a.jsonl", "2026/10/02/rollout-b.jsonl"}) {
		t.Errorf("Discover = %q, %v", got, err)
	}
}

func TestKnownType(t *testing.T) {
	if !rollout.KnownType("session_meta") || rollout.KnownType("hologram") {
		t.Error("KnownType disagrees with the verified format")
	}
}
