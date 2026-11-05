package transcript_test

import (
	"encoding/json"
	"testing"

	"lo0.ai/lo0/internal/claudecode/transcript"
)

func TestAPIErrorShapes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want transcript.APIError
	}{
		{"bare kind", `{"error":"rate_limit"}`, transcript.APIError{Kind: "rate_limit"}},
		{"network down", `{"error":{"formatted":"x","isNetworkDown":true,"connection":null}}`, transcript.APIError{NetworkDown: true}},
		{"connection failed", `{"error":{"connection":{"code":"ECONNRESET"},"isNetworkDown":false}}`, transcript.APIError{NetworkDown: true}},
		{"overloaded", `{"error":{"status":529,"connection":null,"isNetworkDown":false}}`, transcript.APIError{Status: 529}},
		{"absent", `{}`, transcript.APIError{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rec transcript.Record
			if err := json.Unmarshal([]byte(tt.in), &rec); err != nil {
				t.Fatal(err)
			}
			if rec.Error != tt.want {
				t.Errorf("Error = %+v, want %+v", rec.Error, tt.want)
			}
		})
	}
}

func TestToolResultCountsLinesWithoutDecodingThem(t *testing.T) {
	tests := []struct {
		name           string
		in             string
		added, removed int
		path           string
	}{
		{"patch", `{"filePath":"/a.go","structuredPatch":[{"lines":[" ctx","-old","+new","+more"]},{"lines":["-gone"]}]}`, 2, 2, "/a.go"},
		{"created file", `{"type":"create","filePath":"/b.go","content":"one\ntwo\nthree\n","structuredPatch":[]}`, 3, 0, "/b.go"},
		{"created without trailing newline", `{"type":"create","filePath":"/c","content":"a\nb"}`, 2, 0, "/c"},
		{"escaped backslash before n", `{"type":"create","filePath":"/d","content":"a\\nb"}`, 1, 0, "/d"},
		{"empty file", `{"type":"create","filePath":"/e","content":""}`, 0, 0, "/e"},
		{"error string", `"Error: file not found"`, 0, 0, ""},
		{"read result", `{"type":"text","file":{"filePath":"/f","content":"x"}}`, 0, 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var r transcript.ToolResult
			if err := json.Unmarshal([]byte(tt.in), &r); err != nil {
				t.Fatal(err)
			}
			added, removed := r.Changed()
			if added != tt.added || removed != tt.removed || r.FilePath != tt.path {
				t.Errorf("Changed = +%d -%d at %q, want +%d -%d at %q", added, removed, r.FilePath, tt.added, tt.removed, tt.path)
			}
		})
	}
}

func TestContentFindsImagesInToolResults(t *testing.T) {
	var m transcript.Message
	in := `{"content":[{"type":"tool_result","is_error":true,"content":[{"type":"text","text":"x"},{"type":"image","source":{"data":"AAAA"}}]},{"type":"image"}]}`
	if err := json.Unmarshal([]byte(in), &m); err != nil {
		t.Fatal(err)
	}
	results := m.Content.ToolResults()
	if len(results) != 1 || !results[0].IsError || results[0].Content.Images() != 1 {
		t.Errorf("tool results = %+v", results)
	}
	if m.Content.Images() != 1 || m.Content.HasText() {
		t.Errorf("top-level images = %d, text = %v", m.Content.Images(), m.Content.HasText())
	}
}

func TestKnownType(t *testing.T) {
	for _, known := range []string{"assistant", "user", "attachment", "pr-link", ""} {
		if !transcript.KnownType(known) {
			t.Errorf("KnownType(%q) = false", known)
		}
	}
	if transcript.KnownType("hologram") {
		t.Error(`KnownType("hologram") = true`)
	}
}
