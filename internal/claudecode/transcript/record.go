// Package transcript decodes Claude Code session transcripts, the JSONL files
// under ~/.claude/projects.
//
// Only the fields lo0 needs are decoded. Message text, tool inputs other than
// file paths and skill names, tool output and patch contents are skipped,
// both for speed and so that no prompt or code is ever held in memory longer
// than one line.
package transcript

import (
	"encoding/json"
	"time"
)

// Record types and subtypes that carry usage information.
const (
	TypeAssistant  = "assistant"
	TypeUser       = "user"
	TypeSystem     = "system"
	TypeAttachment = "attachment"
	TypePRLink     = "pr-link"

	SubtypeCompactBoundary = "compact_boundary"
	SubtypeAPIError        = "api_error"
)

// StopEndTurn is the stop reason of a response that answers the user rather
// than calling another tool.
const StopEndTurn = "end_turn"

// Record is one line of a transcript.
type Record struct {
	Type        string    `json:"type"`
	Subtype     string    `json:"subtype"`
	SessionID   string    `json:"sessionId"`
	CWD         string    `json:"cwd"`
	Version     string    `json:"version"`
	Entrypoint  string    `json:"entrypoint"`
	IsSidechain bool      `json:"isSidechain"`
	Timestamp   time.Time `json:"timestamp"`
	// IsMeta marks user records Claude Code wrote itself, such as command
	// caveats, rather than something the user typed.
	IsMeta  bool     `json:"isMeta"`
	Message *Message `json:"message"`
	// Effort is the reasoning effort a response was requested at.
	Effort          string           `json:"effort"`
	CompactMetadata *CompactMetadata `json:"compactMetadata"`
	// Error describes a failed request: a kind such as "rate_limit" on the
	// synthetic response Claude Code writes in its place, or the retry
	// details on an api_error system record.
	Error             APIError    `json:"error"`
	IsAPIErrorMessage bool        `json:"isApiErrorMessage"`
	APIErrorStatus    int         `json:"apiErrorStatus"`
	Attachment        *Attachment `json:"attachment"`
	ToolUseResult     ToolResult  `json:"toolUseResult"`
	// PRURL is set on pr-link records, written when a session opens a pull
	// request.
	PRURL string `json:"prUrl"`
}

// Message is the API message inside a user or assistant record.
type Message struct {
	ID         string  `json:"id"`
	Model      string  `json:"model"`
	Content    Content `json:"content"`
	Usage      *Usage  `json:"usage"`
	StopReason string  `json:"stop_reason"`
}

// Usage is the token accounting the API returned for one response.
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	// CacheCreation splits cache writes by lifetime, which are priced
	// differently. Older transcripts omit it.
	CacheCreation *CacheCreation `json:"cache_creation"`
	// OutputDetails splits output tokens; older transcripts omit it.
	OutputDetails *OutputDetails `json:"output_tokens_details"`
	// Speed is "fast" for fast mode, which is priced higher.
	Speed string `json:"speed"`
	// InferenceGeo is "us" when US-only inference was requested.
	InferenceGeo string `json:"inference_geo"`
}

// CacheCreation is the cache write breakdown by time to live.
type CacheCreation struct {
	Ephemeral5m int `json:"ephemeral_5m_input_tokens"`
	Ephemeral1h int `json:"ephemeral_1h_input_tokens"`
}

// OutputDetails is the output token breakdown.
type OutputDetails struct {
	ThinkingTokens int `json:"thinking_tokens"`
}

// ContextTokens is the size of the prompt the model saw: fresh input plus
// everything written to or read from the prompt cache.
func (u Usage) ContextTokens() int {
	return u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
}

// CompactMetadata describes a compaction.
type CompactMetadata struct {
	// Trigger is "auto" or "manual".
	Trigger   string `json:"trigger"`
	PreTokens int    `json:"preTokens"`
}

// Attachment is context Claude Code added to a session, such as a hook's
// result. Only its type and hook event are read.
type Attachment struct {
	Type      string `json:"type"`
	HookEvent string `json:"hookEvent"`
}

// Content is a message body. A bare string, as user prompts and some tool
// results are written, decodes to one text block.
type Content []Block

// UnmarshalJSON implements json.Unmarshaler.
func (c *Content) UnmarshalJSON(data []byte) error {
	switch {
	case len(data) > 0 && data[0] == '"':
		*c = Content{{Type: "text"}}
		return nil
	case len(data) == 0 || data[0] != '[':
		*c = nil
		return nil
	}
	return json.Unmarshal(data, (*[]Block)(c))
}

// Block is one content block. Text is never decoded.
type Block struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Input Input  `json:"input"`
	// Content holds a tool result's own blocks, such as a screenshot.
	Content Content `json:"content"`
	IsError bool    `json:"is_error"`
}

// Input is the subset of a tool_use input that lo0 reads.
type Input struct {
	FilePath     string `json:"file_path"`
	NotebookPath string `json:"notebook_path"`
	// Skill names the skill a Skill tool call loads.
	Skill string `json:"skill"`
}

// ToolUses returns the tool_use blocks in c.
func (c Content) ToolUses() []Block {
	return c.ofType("tool_use")
}

// ToolResults returns the tool_result blocks in c.
func (c Content) ToolResults() []Block {
	return c.ofType("tool_result")
}

// Images counts the image blocks directly in c.
func (c Content) Images() int {
	return len(c.ofType("image"))
}

// HasText reports whether c has a text block: something a person or the
// model wrote, rather than only tool traffic.
func (c Content) HasText() bool {
	return len(c.ofType("text")) > 0
}

func (c Content) ofType(t string) []Block {
	var out []Block
	for _, b := range c {
		if b.Type == t {
			out = append(out, b)
		}
	}
	return out
}

// Path returns the file the tool touched, if any.
func (in Input) Path() string {
	if in.FilePath != "" {
		return in.FilePath
	}
	return in.NotebookPath
}
