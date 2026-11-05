// Package usage turns session summaries into a usage profile: how big
// sessions get, how often they compact, which tools and MCP servers they use,
// which languages and repositories they touch, how requests unfold, what they
// change and what interrupted them.
//
// It knows no coding agent's file formats. Each harness package reads its own
// logs and summarises every session as a Session; everything from here up
// works on those summaries alone, so a new harness needs only a summariser.
//
// The profile holds counts and names only. It never contains prompt text,
// model output or file contents.
package usage

import "time"

// Session is the summary of one session, written by the harness package
// that read it. Fields a harness does not record stay zero, and the report
// leaves them out.
type Session struct {
	// Harness names the coding agent that wrote the session. The pool sets
	// it from the source the session came from.
	Harness string
	ID      string
	CWD     string
	// Version is the newest harness version that wrote to the session.
	Version    string
	Entrypoint string
	Start, End time.Time
	// Turns counts model responses.
	Turns int
	// PeakContext is the largest prompt, in tokens, the model saw.
	PeakContext int
	// ContextWindow is the largest context window the harness reported for
	// the session, or 0 if it does not say.
	ContextWindow   int
	Compactions     int
	AutoCompactions int
	// Tools counts calls to the harness's own tools by name.
	Tools map[string]int
	// MCP counts calls to MCP server tools.
	MCP map[MCPTool]int
	// Languages counts file tool calls by language.
	Languages map[string]int
	// Models counts turns by model.
	Models map[string]int
	// SubagentCalls counts calls to the subagent tool.
	SubagentCalls int
	// DecodeErrors counts log lines that could not be read.
	DecodeErrors int
	// Usage is the tokens the session's API calls used, by model, including
	// any subagent transcripts merged in by the analyzer.
	Usage []ModelUsage

	// Requests has one entry per prompt the session answered.
	Requests []Request
	// Replies has one entry per model response the harness timed, for
	// measuring how fast each model writes.
	Replies []Reply
	// Effort counts turns by requested reasoning effort.
	Effort map[string]int
	// ThinkingTokens and OutputTokens total the session's own responses, so
	// the share spent thinking can be worked out.
	ThinkingTokens, OutputTokens int64
	// ImagesAttached counts images the user put in prompts; ToolImages
	// counts images tools returned, such as browser screenshots.
	ImagesAttached, ToolImages int
	// LimitHits counts responses replaced by a usage or spend limit notice.
	LimitHits int
	// FailedResponses counts responses lost to other API errors.
	FailedResponses int
	// Retries counts requests the harness retried; NetworkRetries those
	// after the connection failed or the machine went offline.
	Retries, NetworkRetries int
	// ToolResults and ToolErrors count tool results and the failed ones.
	ToolResults, ToolErrors int
	// Skills counts skills loaded by name.
	Skills map[string]int
	// Hooks counts hook runs by event.
	Hooks map[string]int
	// Files holds the files the session changed. Paths stay in memory and
	// never reach the profile.
	Files                    map[string]bool
	LinesAdded, LinesRemoved int
	// PullRequests holds the pull requests the session opened.
	PullRequests map[string]bool
	// Responses counts model responses, and WithoutUsage those with no token
	// counts, to notice a changed log format.
	Responses, WithoutUsage int
	// UnknownTypes counts log records of types lo0 has not seen before.
	UnknownTypes map[string]int
}

// Request is one prompt and everything the model did to answer it.
type Request struct {
	ToolCalls int
	// Output is the tokens written across the request's responses.
	Output   int
	Duration time.Duration
}

// Reply is one model response with how long it took: from the last input
// the model was sent, a prompt or a tool result, to the response's last
// record. Time spent running tools is not part of it.
type Reply struct {
	Model    string
	At       time.Time
	Output   int
	Duration time.Duration
	// NewTokens is the input the model read that was not already cached
	// from the previous reply: the prompt or tool results it answers.
	NewTokens int
}

// MCPTool names one tool of one MCP server.
type MCPTool struct {
	Server, Tool string
}
