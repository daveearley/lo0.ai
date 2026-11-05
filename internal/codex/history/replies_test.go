package history_test

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSummarizeTimesReplies(t *testing.T) {
	count := func(at string, total, output int) string {
		return `{"timestamp":"2026-10-02T10:00:` + at + `Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"total_tokens":` +
			itoa(total) + `},"last_token_usage":{"input_tokens":100,"output_tokens":` + itoa(output) + `,"total_tokens":` + itoa(output) + `}}}}`
	}
	log := strings.Join([]string{
		`{"timestamp":"2026-10-02T10:00:00.000Z","type":"turn_context","payload":{"model":"gpt-5.5"}}`,
		`{"timestamp":"2026-10-02T10:00:01.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}}`,
		count("07.000", 500, 300),
		// A repeated count is the same response.
		count("07.100", 500, 300),
		`{"timestamp":"2026-10-02T10:00:30.000Z","type":"response_item","payload":{"type":"function_call_output","call_id":"c1","output":"ok"}}`,
		count("35.000", 900, 250),
		// A response with no input since the last one is not timed.
		count("37.000", 1200, 200),
	}, "\n")
	s := summarize(t, log).Session
	if len(s.Replies) != 2 {
		t.Fatalf("replies = %+v, want two", s.Replies)
	}
	for i, want := range []struct {
		output   int
		duration time.Duration
	}{{300, 6 * time.Second}, {250, 5 * time.Second}} {
		r := s.Replies[i]
		if r.Model != "gpt-5.5" || r.Output != want.output || r.Duration != want.duration {
			t.Errorf("reply %d = %+v, want %d tokens in %v", i, r, want.output, want.duration)
		}
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
