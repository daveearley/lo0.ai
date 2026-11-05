// Package history reads Codex session logs and summarises each session for
// the usage profile, the same summary Claude Code's history produces, so
// everything above usage works for both without knowing which wrote it.
package history

import (
	"errors"
	"iter"
	"strings"
	"time"

	"lo0.ai/lo0/internal/codex/rollout"
	"lo0.ai/lo0/internal/jsonl"
	"lo0.ai/lo0/internal/usage"
	"lo0.ai/lo0/internal/version"
)

// subagentTool is the tool Codex starts a subagent with.
const subagentTool = "spawn_agent"

// Summary is one session's summary and, for a subagent, the session that
// started it.
type Summary struct {
	Session usage.Session
	Parent  string
}

// Summarize reads one session log. Decode errors are counted, not returned;
// a read error is returned with the summary built so far.
func Summarize(records iter.Seq2[rollout.Record, error]) (Summary, error) {
	sum := newSummarizer()
	for rec, err := range records {
		if err != nil {
			if _, ok := errors.AsType[*jsonl.DecodeError](err); ok {
				sum.s.DecodeErrors++
				continue
			}
			return sum.finish(), err
		}
		sum.observe(rec)
	}
	return sum.finish(), nil
}

type summarizer struct {
	s      usage.Session
	parent string
	totals usage.Totals
	// model and effort are the turn context in force.
	model, effort string
	// lastTotal is the running token total of the last count, so repeated
	// counts of the same response are seen once.
	lastTotal int
	limited   bool
	// replaying is true while a forked or subagent session's log repeats
	// the history it started from, which the original session already
	// counted. The copy ends at the session's first turn context, but the
	// session's own first task starts a few records before that, so the
	// records since the last task_started are kept in pending until then.
	replaying bool
	pending   []rollout.Record
	open      *openTask
	// sent is when the model was last sent input, a prompt or tool output,
	// not yet answered; the next response is timed from it.
	sent time.Time
	// Older Codex versions log file changes only as apply_patch input;
	// newer ones also log FileChange items. Both are kept and the richer
	// one used, so a change is never counted twice.
	patches, items changes
}

type openTask struct {
	start     time.Time
	toolCalls int
	output    int
}

type changes struct {
	seen           bool
	files          map[string]bool
	languages      map[string]int
	added, removed int
}

func (c *changes) add(path string, added, removed int) {
	c.seen = true
	c.files[path] = true
	if lang := usage.Language(path); lang != "" {
		c.languages[lang]++
	}
	c.added += added
	c.removed += removed
}

func newChanges() changes {
	return changes{files: map[string]bool{}, languages: map[string]int{}}
}

func newSummarizer() *summarizer {
	return &summarizer{
		s: usage.Session{
			Tools: map[string]int{}, MCP: map[usage.MCPTool]int{}, Languages: map[string]int{}, Models: map[string]int{},
			Effort: map[string]int{}, Skills: map[string]int{}, Hooks: map[string]int{},
			Files: map[string]bool{}, PullRequests: map[string]bool{}, UnknownTypes: map[string]int{},
		},
		totals:  usage.Totals{},
		patches: newChanges(),
		items:   newChanges(),
	}
}

func (sum *summarizer) finish() Summary {
	s := &sum.s
	s.Usage = sum.totals.Usages()
	c := sum.patches
	if sum.items.seen {
		c = sum.items
	}
	s.Files, s.Languages = c.files, c.languages
	s.LinesAdded, s.LinesRemoved = c.added, c.removed
	return Summary{Session: *s, Parent: sum.parent}
}

func (sum *summarizer) observe(rec rollout.Record) {
	s := &sum.s
	if !rec.Timestamp.IsZero() {
		if s.Start.IsZero() || rec.Timestamp.Before(s.Start) {
			s.Start = rec.Timestamp
		}
		if rec.Timestamp.After(s.End) {
			s.End = rec.Timestamp
		}
	}
	if !rollout.KnownType(rec.Type) {
		s.UnknownTypes[rec.Type]++
	}
	p := rec.Payload
	if rec.Type == rollout.TypeSessionMeta {
		if s.ID == "" {
			s.ID, s.CWD, s.Entrypoint, sum.parent = p.ID, p.CWD, p.Originator, p.Source.Parent
			sum.replaying = p.ForkedFrom != "" || p.Source.Parent != ""
		}
		if v := version.Parse(p.CLIVersion); version.Compare(v, s.Version) > 0 {
			s.Version = v
		}
		return
	}
	if sum.replaying {
		switch {
		case rec.Type == rollout.TypeTurnContext:
			sum.replaying = false
			pending := sum.pending
			sum.pending = nil
			for _, r := range pending {
				sum.handle(r)
			}
		case rec.Type == rollout.TypeEventMsg && p.Type == rollout.EventTaskStarted:
			sum.pending = append(sum.pending[:0], rec)
			return
		default:
			if sum.pending != nil {
				sum.pending = append(sum.pending, rec)
			}
			return
		}
	}
	sum.handle(rec)
}

// handle counts one record of the session's own, after observe has dated it
// and set replayed history aside.
func (sum *summarizer) handle(rec rollout.Record) {
	s, p := &sum.s, rec.Payload
	switch rec.Type {
	case rollout.TypeTurnContext:
		if p.Model != "" {
			sum.model = p.Model
		}
		sum.effort = p.Effort
	case rollout.TypeCompacted:
		s.Compactions++
	case rollout.TypeResponseItem:
		sum.responseItem(rec.Timestamp, p)
	case rollout.TypeEventMsg:
		sum.event(rec.Timestamp, p)
	}
}

func (sum *summarizer) responseItem(at time.Time, p rollout.Payload) {
	s := &sum.s
	switch p.Type {
	case rollout.ItemMessage:
		if p.Role == "user" {
			s.ImagesAttached += p.Content.Images()
			sum.sent = at
		}
	case rollout.ItemFunctionCall, rollout.ItemCustomToolCall:
		if server, ok := strings.CutPrefix(p.Namespace, "mcp__"); ok && server != "" {
			s.MCP[usage.MCPTool{Server: server, Tool: p.Name}]++
		} else {
			s.Tools[p.Name]++
		}
		if p.Name == subagentTool {
			s.SubagentCalls++
		}
		for _, f := range p.Input.Files {
			sum.patches.add(f, 0, 0)
		}
		sum.patches.added += p.Input.Added
		sum.patches.removed += p.Input.Removed
		if sum.open != nil {
			sum.open.toolCalls++
		}
	case rollout.ItemFunctionOutput, rollout.ItemCustomToolOutput:
		s.ToolImages += p.Output.Images()
		sum.sent = at
	}
}

func (sum *summarizer) event(at time.Time, p rollout.Payload) {
	s := &sum.s
	switch p.Type {
	case rollout.EventTokenCount:
		sum.tokens(at, p)
	case rollout.EventTaskStarted:
		sum.open = &openTask{start: at}
	case rollout.EventTaskComplete:
		if p.Error.Failed() {
			if p.Error.Info == rollout.ErrorUsageLimit {
				s.LimitHits++
			} else {
				s.FailedResponses++
			}
		}
		if t := sum.open; t != nil {
			sum.open = nil
			req := usage.Request{ToolCalls: t.toolCalls, Output: t.output}
			if !t.start.IsZero() && at.After(t.start) {
				req.Duration = at.Sub(t.start)
			}
			s.Requests = append(s.Requests, req)
		}
	case rollout.EventItemCompleted:
		item := p.Item
		if item == nil {
			return
		}
		switch item.Type {
		case rollout.ItemCommandExecution, rollout.ItemMcpToolCall:
			s.ToolResults++
			if item.Status == rollout.StatusFailed {
				s.ToolErrors++
			}
		case rollout.ItemFileChange:
			for path, c := range item.Changes {
				sum.items.add(path, c.Diff.Added, c.Diff.Removed)
			}
		}
	}
}

// tokens counts one response. Codex logs a running total with each count
// and sometimes repeats a count, so only a count that moves the total is a
// new response. A total that falls means the log restarted it, as forked
// sessions do.
func (sum *summarizer) tokens(at time.Time, p rollout.Payload) {
	s := &sum.s
	if r := p.RateLimits; r != nil {
		reached := r.ReachedType != nil
		if reached && !sum.limited {
			s.LimitHits++
		}
		sum.limited = reached
	}
	info := p.Info
	if info == nil {
		return
	}
	s.ContextWindow = max(s.ContextWindow, info.ContextWindow)
	if info.Total.TotalTokens == sum.lastTotal {
		return
	}
	sum.lastTotal = info.Total.TotalTokens
	last := info.Last
	s.Turns++
	s.Responses++
	if sum.model != "" {
		s.Models[sum.model]++
	}
	if sum.effort != "" {
		s.Effort[sum.effort]++
	}
	s.PeakContext = max(s.PeakContext, last.InputTokens)
	s.ThinkingTokens += int64(last.ReasoningOutputTokens)
	s.OutputTokens += int64(last.OutputTokens)
	if sum.open != nil {
		sum.open.output += last.OutputTokens
	}
	if !sum.sent.IsZero() && sum.model != "" && at.After(sum.sent) {
		s.Replies = append(s.Replies, usage.Reply{
			Model: sum.model, At: at, Output: last.OutputTokens, Duration: at.Sub(sum.sent),
			NewTokens: max(last.InputTokens-last.CachedInputTokens, 0),
		})
	}
	sum.sent = time.Time{}
	fresh := max(last.InputTokens-last.CachedInputTokens-last.CacheWriteInputTokens, 0)
	sum.totals.Add(
		usage.Variant{Model: sum.model, LongPrompt: last.InputTokens > usage.LongPromptTokens},
		usage.Tokens{
			Input:        int64(fresh),
			CacheWrite5m: int64(last.CacheWriteInputTokens),
			CacheRead:    int64(last.CachedInputTokens),
			Output:       int64(last.OutputTokens),
		},
	)
}
