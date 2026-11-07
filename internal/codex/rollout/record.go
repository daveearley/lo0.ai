// Package rollout decodes Codex session logs, the rollout-*.jsonl files under
// ~/.codex/sessions.
//
// Only the fields lo0 needs are decoded. Prompts, model output, reasoning,
// command output and code are skipped; patches and diffs are reduced to line
// counts and file paths by scanning them without building their text.
package rollout

import (
	"bytes"
	"encoding/json"
	"io"
	"iter"
	"time"

	"lo0.ai/lo0/internal/jsonl"
)

// Record types.
const (
	TypeSessionMeta  = "session_meta"
	TypeTurnContext  = "turn_context"
	TypeResponseItem = "response_item"
	TypeEventMsg     = "event_msg"
	TypeCompacted    = "compacted"
)

// Payload types inside response items and events.
const (
	ItemMessage          = "message"
	ItemFunctionCall     = "function_call"
	ItemCustomToolCall   = "custom_tool_call"
	ItemFunctionOutput   = "function_call_output"
	ItemCustomToolOutput = "custom_tool_call_output"
	ItemReasoning        = "reasoning"
	ItemWebSearchCall    = "web_search_call"

	EventTokenCount    = "token_count"
	EventTaskStarted   = "task_started"
	EventTaskComplete  = "task_complete"
	EventItemCompleted = "item_completed"
)

// ErrorUsageLimit is the error a task ends with when the plan's limit is hit.
const ErrorUsageLimit = "usage_limit_exceeded"

// knownTypes are the record types Codex wrote up to VerifiedVersion.
var knownTypes = map[string]bool{
	TypeSessionMeta: true, TypeTurnContext: true, TypeResponseItem: true, TypeEventMsg: true,
	TypeCompacted: true, "token_usage_record": true, "world_state": true,
}

// KnownType reports whether t is a record type lo0 has seen Codex write.
func KnownType(t string) bool { return knownTypes[t] }

// Record is one line of a session log.
type Record struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	Payload   Payload   `json:"payload"`
}

// Payload holds the fields lo0 reads from every kind of record. Each record
// type fills only its own.
type Payload struct {
	Type string `json:"type"`

	// Session metadata.
	ID         string `json:"id"`
	CWD        string `json:"cwd"`
	Originator string `json:"originator"`
	CLIVersion string `json:"cli_version"`
	Source     Source `json:"source"`
	// ForkedFrom is the session a forked session copied its history from.
	ForkedFrom string `json:"forked_from_id"`

	// Turn context: the model and effort in force until the next one.
	Model  string `json:"model"`
	Effort string `json:"effort"`

	// Response items.
	Role      string  `json:"role"`
	Content   Content `json:"content"`
	Name      string  `json:"name"`
	Namespace string  `json:"namespace"`
	Input     Patch   `json:"input"`
	Output    Content `json:"output"`

	// Events.
	Info       *TokenInfo  `json:"info"`
	RateLimits *RateLimits `json:"rate_limits"`
	Error      TaskError   `json:"error"`
	Item       *Item       `json:"item"`
}

// Source says what started a session. Subagent sessions name their parent.
type Source struct {
	// Parent is the parent session's ID for a subagent, otherwise empty.
	Parent string
}

// UnmarshalJSON implements json.Unmarshaler. A plain source such as "cli"
// decodes to the zero value.
func (s *Source) UnmarshalJSON(data []byte) error {
	*s = Source{}
	if len(data) == 0 || data[0] != '{' {
		return nil
	}
	var obj struct {
		Subagent struct {
			ThreadSpawn struct {
				Parent string `json:"parent_thread_id"`
			} `json:"thread_spawn"`
		} `json:"subagent"`
	}
	// An unfamiliar source shape is not a broken record, so it decodes to
	// no parent rather than failing the line.
	if json.Unmarshal(data, &obj) == nil {
		s.Parent = obj.Subagent.ThreadSpawn.Parent
	}
	return nil
}

// Content is a list of blocks, of which only the types are read. A string,
// as tool outputs often are, decodes to no blocks.
type Content []Block

// Block is one content block.
type Block struct {
	Type string `json:"type"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (c *Content) UnmarshalJSON(data []byte) error {
	*c = nil
	if len(data) == 0 || data[0] != '[' {
		return nil
	}
	return json.Unmarshal(data, (*[]Block)(c))
}

// Images counts the image blocks in c.
func (c Content) Images() int {
	n := 0
	for _, b := range c {
		if b.Type == "input_image" {
			n++
		}
	}
	return n
}

// TokenUsage is the token accounting for a response or a running total.
// InputTokens includes the cached and cache-write tokens.
type TokenUsage struct {
	InputTokens           int `json:"input_tokens"`
	CachedInputTokens     int `json:"cached_input_tokens"`
	CacheWriteInputTokens int `json:"cache_write_input_tokens"`
	OutputTokens          int `json:"output_tokens"`
	// ReasoningOutputTokens is the part of OutputTokens spent reasoning.
	ReasoningOutputTokens int `json:"reasoning_output_tokens"`
	TotalTokens           int `json:"total_tokens"`
}

// TokenInfo is a token count event's usage.
type TokenInfo struct {
	Total         TokenUsage `json:"total_token_usage"`
	Last          TokenUsage `json:"last_token_usage"`
	ContextWindow int        `json:"model_context_window"`
}

// RateLimits is the plan's limit state Codex reports with each token count.
type RateLimits struct {
	// ReachedType is set while a limit is reached.
	ReachedType *string `json:"rate_limit_reached_type"`
	PlanType    string  `json:"plan_type"`
}

// TaskError is the error a task ended with. Only its kind is read.
type TaskError struct {
	Info string
}

// UnmarshalJSON implements json.Unmarshaler.
func (e *TaskError) UnmarshalJSON(data []byte) error {
	*e = TaskError{}
	if len(data) == 0 || data[0] != '{' {
		return nil
	}
	var obj struct {
		Info string `json:"codex_error_info"`
	}
	// Any error object means the task failed, even one whose shape is new.
	_ = json.Unmarshal(data, &obj)
	*e = TaskError{Info: obj.Info}
	if e.Info == "" {
		e.Info = "unknown"
	}
	return nil
}

// Failed reports whether the task ended with an error.
func (e TaskError) Failed() bool { return e.Info != "" }

// Item is a completed thread item: a command, an MCP call or a file change.
type Item struct {
	Type    string            `json:"type"`
	Status  string            `json:"status"`
	Changes map[string]Change `json:"changes"`
}

// Item types and statuses lo0 reads.
const (
	ItemCommandExecution = "CommandExecution"
	ItemMcpToolCall      = "McpToolCall"
	ItemFileChange       = "FileChange"
	StatusFailed         = "failed"
)

// Change is one file's change in a FileChange item, reduced to its diff's
// line counts.
type Change struct {
	Diff Diff `json:"unified_diff"`
}

// Diff counts a unified diff's added and removed lines without decoding it.
type Diff struct {
	Added, Removed int
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *Diff) UnmarshalJSON(data []byte) error {
	*d = Diff{}
	scanLines(data, func(line []byte) {
		switch {
		case bytes.HasPrefix(line, []byte("+++")), bytes.HasPrefix(line, []byte("---")):
		case bytes.HasPrefix(line, []byte("+")):
			d.Added++
		case bytes.HasPrefix(line, []byte("-")):
			d.Removed++
		}
	})
	return nil
}

// Records yields each record in r. A line that fails to decode yields a
// *jsonl.DecodeError and iteration continues.
func Records(r io.Reader) iter.Seq2[Record, error] {
	return jsonl.Records[Record](r)
}
