package history_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"lo0.ai/lo0/internal/claudecode/history"
	"lo0.ai/lo0/internal/claudecode/transcript"
	"lo0.ai/lo0/internal/usage"
)

func TestSummarizeTimesReplies(t *testing.T) {
	log := strings.Join([]string{
		`{"type":"user","message":{"role":"user","content":"Fix it"},"sessionId":"s","timestamp":"2026-10-01T09:00:00.000Z"}`,
		// One reply in two records: it ends at the last one, with its final token count.
		`{"type":"assistant","message":{"id":"a","model":"claude-opus-5-5","content":[{"type":"text","text":"x"}],"usage":{"output_tokens":100}},"sessionId":"s","timestamp":"2026-10-01T09:00:03.000Z"}`,
		`{"type":"assistant","message":{"id":"a","model":"claude-opus-5-5","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{}}],"usage":{"output_tokens":300}},"sessionId":"s","timestamp":"2026-10-01T09:00:05.000Z"}`,
		// The tool ran for a minute; the next reply is timed from its result.
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]},"sessionId":"s","timestamp":"2026-10-01T09:01:05.000Z"}`,
		`{"type":"user","isSidechain":true,"message":{"role":"user","content":"subagent task"},"sessionId":"s","timestamp":"2026-10-01T09:01:06.000Z"}`,
		`{"type":"assistant","isSidechain":true,"message":{"id":"side","model":"claude-haiku-4-5","content":[{"type":"text","text":"x"}],"usage":{"output_tokens":500}},"sessionId":"s","timestamp":"2026-10-01T09:01:07.000Z"}`,
		`{"type":"assistant","message":{"id":"b","model":"claude-opus-5-5","content":[{"type":"text","text":"done"}],"usage":{"output_tokens":400},"stop_reason":"end_turn"},"sessionId":"s","timestamp":"2026-10-01T09:01:09.000Z"}`,
		`{"type":"assistant","message":{"id":"c","model":"<synthetic>","content":[{"type":"text","text":"error"}],"usage":{"output_tokens":0}},"sessionId":"s","timestamp":"2026-10-01T09:01:10.000Z"}`,
	}, "\n")
	s, err := history.Summarize(transcript.Records(strings.NewReader(log)))
	if err != nil {
		t.Fatal(err)
	}
	at := func(min, sec int) time.Time { return time.Date(2026, 10, 1, 9, min, sec, 0, time.UTC) }
	want := []usage.Reply{
		{Model: "claude-opus-5-5", At: at(0, 5), Output: 300, Duration: 5 * time.Second},
		{Model: "claude-opus-5-5", At: at(1, 9), Output: 400, Duration: 4 * time.Second},
	}
	if !slices.EqualFunc(s.Replies, want, func(a, b usage.Reply) bool {
		return a.Model == b.Model && a.At.Equal(b.At) && a.Output == b.Output && a.Duration == b.Duration
	}) {
		t.Errorf("replies = %+v\nwant %+v", s.Replies, want)
	}
}
