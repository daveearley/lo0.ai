package transcript_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"testing/iotest"

	"lo0.ai/lo0/internal/claudecode/transcript"
)

func open(t *testing.T, name string) *os.File {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func TestRecords(t *testing.T) {
	var (
		recs      []transcript.Record
		decodeErr *transcript.DecodeError
	)
	for rec, err := range transcript.Records(open(t, "session.jsonl")) {
		if err != nil {
			if de, ok := errors.AsType[*transcript.DecodeError](err); ok {
				decodeErr = de
				continue
			}
			t.Fatalf("Records: %v", err)
		}
		recs = append(recs, rec)
	}
	if len(recs) != 8 {
		t.Fatalf("got %d records, want 8", len(recs))
	}
	if decodeErr == nil || decodeErr.Line != 9 {
		t.Errorf("DecodeError = %v, want one for the truncated line 9", decodeErr)
	}

	user := recs[1]
	if user.Type != transcript.TypeUser || user.CWD != "/home/u/code/app" || user.Version != "2.4.1" || !user.Message.Content.HasText() || len(user.Message.Content) != 1 {
		t.Errorf("string-content user record = %+v", user)
	}

	first := recs[2]
	if first.Message.Usage.ContextTokens() != 12005 {
		t.Errorf("ContextTokens = %d, want 12005", first.Message.Usage.ContextTokens())
	}
	uses := first.Message.Content.ToolUses()
	if len(uses) != 1 || uses[0].Name != "Read" || uses[0].Input.Path() != "/home/u/code/app/auth.go" {
		t.Errorf("tool uses = %+v", uses)
	}

	compact := recs[5]
	if compact.Subtype != transcript.SubtypeCompactBoundary || compact.CompactMetadata.Trigger != "auto" || compact.CompactMetadata.PreTokens != 180000 {
		t.Errorf("compact record = %+v", compact)
	}
}

func TestRecordsHandlesLongLines(t *testing.T) {
	long := `{"type":"user","message":{"content":"` + strings.Repeat("x", 3<<20) + `"}}` + "\n" + `{"type":"assistant"}`
	var types []string
	for rec, err := range transcript.Records(iotest.OneByteReader(strings.NewReader(long))) {
		if err != nil {
			t.Fatalf("Records: %v", err)
		}
		types = append(types, rec.Type)
	}
	if !slices.Equal(types, []string{"user", "assistant"}) {
		t.Errorf("types = %v", types)
	}
}

func TestRecordsStopsOnReadError(t *testing.T) {
	boom := errors.New("disk gone")
	r := io.MultiReader(strings.NewReader(`{"type":"user"}`+"\n"), iotest.ErrReader(boom))
	var errs []error
	for _, err := range transcript.Records(r) {
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) != 1 || !errors.Is(errs[0], boom) {
		t.Errorf("errors = %v, want exactly %v", errs, boom)
	}
}

func TestRecordsStopsWhenConsumerBreaks(t *testing.T) {
	n := 0
	for _, err := range transcript.Records(open(t, "session.jsonl")) {
		if err != nil {
			continue
		}
		n++
		if n == 2 {
			break
		}
	}
	if n != 2 {
		t.Errorf("iterated %d records after break, want 2", n)
	}
}

func TestDiscover(t *testing.T) {
	fsys := fstest.MapFS{
		"-home-u-app/a1.jsonl":                                  {},
		"-home-u-app/a1/subagents/agent-x.jsonl":                {},
		"-home-u-app/a1/subagents/workflows/wf_1/agent-y.jsonl": {},
		"-home-u-app/a2.jsonl":                                  {},
		"-home-u-app/gone/subagents/agent-z.jsonl":              {},
		"-home-u-app/a2/tool-results/big.txt":                   {},
		"-home-u-lib/b1.jsonl":                                  {},
	}
	got, err := transcript.Discover(fsys)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	want := []transcript.Session{
		{ID: "a1", Dir: "-home-u-app", Main: "-home-u-app/a1.jsonl", Subagents: []string{
			"-home-u-app/a1/subagents/agent-x.jsonl",
			"-home-u-app/a1/subagents/workflows/wf_1/agent-y.jsonl",
		}},
		{ID: "a2", Dir: "-home-u-app", Main: "-home-u-app/a2.jsonl"},
		{ID: "b1", Dir: "-home-u-lib", Main: "-home-u-lib/b1.jsonl"},
	}
	if len(got) != len(want) {
		t.Fatalf("Discover = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i].ID != want[i].ID || got[i].Main != want[i].Main || !slices.Equal(got[i].Subagents, want[i].Subagents) {
			t.Errorf("session %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func BenchmarkRecords(b *testing.B) {
	data, err := os.ReadFile(filepath.Join("testdata", "session.jsonl"))
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(data)))
	for b.Loop() {
		n := 0
		for _, err := range transcript.Records(strings.NewReader(string(data))) {
			if err == nil {
				n++
			}
		}
		if n == 0 {
			b.Fatal("no records decoded")
		}
	}
}
