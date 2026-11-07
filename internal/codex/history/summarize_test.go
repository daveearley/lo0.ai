package history_test

import (
	"maps"
	"strconv"
	"strings"
	"testing"
	"time"

	"lo0.ai/lo0/internal/usage"
)

func record(at, typ, payload string) string {
	return `{"timestamp":"2026-10-02T10:00:` + at + `Z","type":"` + typ + `","payload":` + payload + `}`
}

func count(at string, total, output int) string {
	return record(at, "event_msg", `{"type":"token_count","info":{"total_token_usage":{"total_tokens":`+
		itoa(total)+`},"last_token_usage":{"input_tokens":100,"output_tokens":`+itoa(output)+`,"total_tokens":`+itoa(output)+`}}}`)
}

func prompt(at string) string {
	return record(at, "response_item", `{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}`)
}

func reasoning(at string) string {
	return record(at, "response_item", `{"type":"reasoning","summary":[]}`)
}

func answer(at string) string {
	return record(at, "response_item", `{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]}`)
}

func call(at string) string {
	return record(at, "response_item", `{"type":"custom_tool_call","name":"exec","call_id":"c","input":"ls"}`)
}

func callOutput(at string) string {
	return record(at, "response_item", `{"type":"custom_tool_call_output","call_id":"c","output":"ok"}`)
}

func TestSummarizeTimesReplies(t *testing.T) {
	type reply struct {
		output   int
		duration time.Duration
	}
	for _, tt := range []struct {
		name string
		log  []string
		want []reply
	}{
		{
			name: "count after the response, up to Codex 0.135",
			log: []string{
				prompt("01.000"),
				reasoning("04.000"),
				call("07.000"),
				count("07.050", 500, 300),
				// A repeated count is the same response.
				count("07.100", 500, 300),
				callOutput("30.000"),
				reasoning("33.000"),
				answer("35.000"),
				count("35.010", 900, 250),
				// A response with no input since the last one is not timed.
				answer("37.000"),
				count("37.010", 1200, 200),
			},
			want: []reply{{300, 6 * time.Second}, {250, 5 * time.Second}},
		},
		{
			name: "count after the tool output, from Codex 0.153",
			log: []string{
				prompt("01.000"),
				reasoning("04.000"),
				call("07.000"),
				record("07.040", "token_usage_record", `{}`),
				callOutput("30.000"),
				count("30.001", 500, 300),
				call("34.000"),
				record("34.040", "token_usage_record", `{}`),
				callOutput("36.000"),
				callOutput("36.500"),
				count("36.501", 900, 250),
				answer("40.000"),
				count("40.010", 1200, 200),
			},
			want: []reply{{300, 6 * time.Second}, {250, 4 * time.Second}, {200, 3500 * time.Millisecond}},
		},
		{
			name: "a count with no output since the input is not timed",
			log: []string{
				prompt("01.000"),
				count("07.000", 500, 300),
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			log := append([]string{record("00.000", "turn_context", `{"model":"gpt-5.5"}`)}, tt.log...)
			s := summarize(t, strings.Join(log, "\n")).Session
			if len(s.Replies) != len(tt.want) {
				t.Fatalf("replies = %+v, want %d", s.Replies, len(tt.want))
			}
			for i, want := range tt.want {
				r := s.Replies[i]
				if r.Model != "gpt-5.5" || r.Output != want.output || r.Duration != want.duration {
					t.Errorf("reply %d = %+v, want %d tokens in %v", i, r, want.output, want.duration)
				}
			}
		})
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestSummarizeNamesMCPTools(t *testing.T) {
	mcp := func(namespace, name string) string {
		return record("01.000", "response_item", `{"type":"function_call","namespace":"`+namespace+`","name":"`+name+`","arguments":"{}"}`)
	}
	s := summarize(t, strings.Join([]string{
		mcp("mcp__cua_repl", "js"),
		mcp("mcp__codex_apps__gmail", "_search_emails"),
		mcp("mcp__", "orphan"),
	}, "\n")).Session
	want := map[usage.MCPTool]int{{Server: "cua_repl", Tool: "js"}: 1, {Server: "codex_apps", Tool: "gmail_search_emails"}: 1}
	if !maps.Equal(s.MCP, want) {
		t.Errorf("MCP = %v, want %v", s.MCP, want)
	}
	if s.Tools["orphan"] != 1 {
		t.Errorf("Tools = %v, want a call with no server counted as built in", s.Tools)
	}
}

func TestSummarizeTellsAutomaticCompactions(t *testing.T) {
	started := record("00.000", "event_msg", `{"type":"task_started"}`)
	compacted := record("02.000", "compacted", `{"message":""}`)
	s := summarize(t, strings.Join([]string{
		// A manual /compact is a task with nothing else in it.
		started, compacted,
		record("03.000", "event_msg", `{"type":"task_complete"}`),
		// Codex compacts on its own mid-task, once the context fills.
		started, prompt("00.500"), call("01.500"), callOutput("01.900"), compacted,
	}, "\n")).Session
	if s.Compactions != 2 || s.AutoCompactions != 1 {
		t.Errorf("compactions %d, automatic %d; want 2 and 1", s.Compactions, s.AutoCompactions)
	}
}
