package history

import (
	"errors"
	"iter"
	"maps"
	"slices"
	"strings"
	"time"

	"lo0.ai/lo0/internal/claudecode/transcript"
	"lo0.ai/lo0/internal/usage"
	"lo0.ai/lo0/internal/version"
)

// billed is one response's usage, kept until the transcript ends because
// Claude Code repeats a response's record for every content block.
type billed struct {
	variant  usage.Variant
	tokens   usage.Tokens
	thinking int64
}

// subagentTools are the names the subagent tool has had.
var subagentTools = map[string]bool{"Agent": true, "Task": true}

// syntheticModel marks responses Claude Code wrote itself, such as errors.
const syntheticModel = "<synthetic>"

// Summarize reads one transcript. Decode errors are counted, not returned; a
// read error is returned with the summary built so far.
func Summarize(records iter.Seq2[transcript.Record, error]) (usage.Session, error) {
	s, _, err := summarize(records, nil)
	return s, err
}

// summarize is Summarize that also returns the IDs of the responses it
// billed, and bills none for which billed reports true, because a resumed
// or forked session's transcript repeats responses another one already has.
func summarize(records iter.Seq2[transcript.Record, error], billedElsewhere func(id string) bool) (usage.Session, []string, error) {
	sum := newSummarizer()
	sum.billedElsewhere = billedElsewhere
	for rec, err := range records {
		if err != nil {
			if _, ok := errors.AsType[*transcript.DecodeError](err); ok {
				sum.s.DecodeErrors++
				continue
			}
			return sum.finish(), sum.ids(), err
		}
		sum.observe(rec)
	}
	return sum.finish(), sum.ids(), nil
}

// summarizer holds the state of one transcript read.
type summarizer struct {
	s         usage.Session
	seen      map[string]bool
	responses map[string]billed
	// billedElsewhere, if set, reports responses another transcript bills.
	billedElsewhere func(id string) bool
	// open is the request being answered, if any.
	open *openRequest
	// sent is when the model was last sent input, a prompt or tool
	// results, on the main chain; replies are timed from it.
	sent    time.Time
	replies map[string]*timedReply
}

// timedReply is one response being timed. Its records may span several
// timestamps, one per content block, and the last one ends it.
type timedReply struct {
	model      string
	start, end time.Time
	output     int
	newTokens  int
}

type openRequest struct {
	start     time.Time
	toolCalls int
	messages  map[string]bool
}

func newSummarizer() *summarizer {
	return &summarizer{
		s: usage.Session{
			Tools: map[string]int{}, MCP: map[usage.MCPTool]int{}, Languages: map[string]int{}, Models: map[string]int{},
			Effort: map[string]int{}, Skills: map[string]int{}, Hooks: map[string]int{},
			Files: map[string]bool{}, PullRequests: map[string]bool{}, UnknownTypes: map[string]int{},
		},
		seen:      map[string]bool{},
		responses: map[string]billed{},
		replies:   map[string]*timedReply{},
	}
}

func (sum *summarizer) ids() []string {
	return slices.Sorted(maps.Keys(sum.responses))
}

func (sum *summarizer) finish() usage.Session {
	totals := usage.Totals{}
	for _, b := range sum.responses {
		totals.Add(b.variant, b.tokens)
		sum.s.ThinkingTokens += b.thinking
		sum.s.OutputTokens += b.tokens.Output
	}
	sum.s.Usage = totals.Usages()
	replies := make([]usage.Reply, 0, len(sum.replies))
	for _, r := range sum.replies {
		if r.end.After(r.start) {
			replies = append(replies, usage.Reply{Model: r.model, At: r.end, Output: r.output, Duration: r.end.Sub(r.start), NewTokens: r.newTokens})
		}
	}
	slices.SortFunc(replies, func(a, b usage.Reply) int { return a.At.Compare(b.At) })
	sum.s.Replies = replies
	return sum.s
}

// time records an assistant record against its response's timing. Only
// the main chain is timed: subagents run alongside it, so their records
// interleave with the main chain's input.
func (sum *summarizer) time(rec transcript.Record) {
	m := rec.Message
	if m.ID == "" || m.Usage == nil || m.Model == syntheticModel || rec.Timestamp.IsZero() {
		return
	}
	r := sum.replies[m.ID]
	if r == nil {
		if sum.sent.IsZero() {
			return
		}
		r = &timedReply{model: m.Model, start: sum.sent}
		sum.replies[m.ID] = r
	}
	if rec.Timestamp.After(r.end) {
		r.end = rec.Timestamp
	}
	r.output = max(r.output, m.Usage.OutputTokens)
	r.newTokens = max(r.newTokens, m.Usage.InputTokens+m.Usage.CacheCreationInputTokens)
}

func (sum *summarizer) observe(rec transcript.Record) {
	s := &sum.s
	if s.ID == "" {
		s.ID = rec.SessionID
	}
	if s.CWD == "" {
		s.CWD = rec.CWD
	}
	if rec.Version != "" && version.Compare(rec.Version, s.Version) > 0 {
		s.Version = rec.Version
	}
	if s.Entrypoint == "" {
		s.Entrypoint = rec.Entrypoint
	}
	if !rec.Timestamp.IsZero() {
		if s.Start.IsZero() || rec.Timestamp.Before(s.Start) {
			s.Start = rec.Timestamp
		}
		if rec.Timestamp.After(s.End) {
			s.End = rec.Timestamp
		}
	}
	if !transcript.KnownType(rec.Type) {
		s.UnknownTypes[rec.Type]++
	}

	switch rec.Type {
	case transcript.TypeSystem:
		sum.system(rec)
	case transcript.TypeAssistant:
		sum.assistant(rec)
	case transcript.TypeUser:
		sum.user(rec)
	case transcript.TypeAttachment:
		if a := rec.Attachment; a != nil && strings.HasPrefix(a.Type, "hook_") && a.Type != "hook_additional_context" && a.HookEvent != "" {
			s.Hooks[a.HookEvent]++
		}
	case transcript.TypePRLink:
		if rec.PRURL != "" {
			s.PullRequests[rec.PRURL] = true
		}
	}
}

func (sum *summarizer) system(rec transcript.Record) {
	s := &sum.s
	switch rec.Subtype {
	case transcript.SubtypeCompactBoundary:
		s.Compactions++
		if rec.CompactMetadata != nil && rec.CompactMetadata.Trigger == "auto" {
			s.AutoCompactions++
		}
	case transcript.SubtypeAPIError:
		s.Retries++
		if rec.Error.NetworkDown {
			s.NetworkRetries++
		}
	}
}

func (sum *summarizer) assistant(rec transcript.Record) {
	s := &sum.s
	if rec.Message == nil {
		return
	}
	m := rec.Message
	if rec.IsAPIErrorMessage {
		if rec.Error.Kind == transcript.ErrorRateLimit || rec.APIErrorStatus == 429 {
			s.LimitHits++
		} else {
			s.FailedResponses++
		}
		return
	}
	if m.Model != syntheticModel {
		s.Responses++
		if m.Usage == nil {
			s.WithoutUsage++
		}
	}
	if m.Usage != nil && m.ID != "" && m.Model != syntheticModel && (sum.billedElsewhere == nil || !sum.billedElsewhere(m.ID)) {
		b := billed{variant: variantOf(m.Model, *m.Usage), tokens: tokensOf(*m.Usage)}
		if d := m.Usage.OutputDetails; d != nil {
			b.thinking = int64(d.ThinkingTokens)
		}
		sum.responses[m.ID] = b
	}
	if rec.IsSidechain {
		return
	}
	sum.time(rec)
	if m.Usage != nil {
		s.PeakContext = max(s.PeakContext, m.Usage.ContextTokens())
	}
	if m.ID != "" && !sum.seen[m.ID] {
		sum.seen[m.ID] = true
		s.Turns++
		if m.Model != "" && m.Model != syntheticModel {
			s.Models[m.Model]++
		}
		if rec.Effort != "" {
			s.Effort[rec.Effort]++
		}
	}
	uses := m.Content.ToolUses()
	for _, use := range uses {
		if call, ok := mcpTool(use.Name); ok {
			s.MCP[call]++
		} else {
			s.Tools[use.Name]++
		}
		if subagentTools[use.Name] {
			s.SubagentCalls++
		}
		if use.Name == "Skill" && use.Input.Skill != "" {
			s.Skills[use.Input.Skill]++
		}
		if lang := usage.Language(use.Input.Path()); lang != "" {
			s.Languages[lang]++
		}
	}
	if r := sum.open; r != nil {
		r.toolCalls += len(uses)
		if m.ID != "" {
			r.messages[m.ID] = true
		}
		if m.StopReason == transcript.StopEndTurn {
			sum.closeRequest(rec.Timestamp)
		}
	}
}

func (sum *summarizer) closeRequest(end time.Time) {
	r := sum.open
	sum.open = nil
	req := usage.Request{ToolCalls: r.toolCalls}
	for id := range r.messages {
		req.Output += int(sum.responses[id].tokens.Output)
	}
	if !r.start.IsZero() && end.After(r.start) {
		req.Duration = end.Sub(r.start)
	}
	sum.s.Requests = append(sum.s.Requests, req)
}

func (sum *summarizer) user(rec transcript.Record) {
	s := &sum.s
	if rec.Message == nil || rec.IsSidechain {
		return
	}
	if !rec.Timestamp.IsZero() {
		sum.sent = rec.Timestamp
	}
	c := rec.Message.Content
	for _, res := range c.ToolResults() {
		s.ToolResults++
		if res.IsError {
			s.ToolErrors++
		}
		s.ToolImages += res.Content.Images()
	}
	if r := rec.ToolUseResult; r.FilePath != "" {
		added, removed := r.Changed()
		if added+removed > 0 {
			s.Files[r.FilePath] = true
			s.LinesAdded += added
			s.LinesRemoved += removed
		}
	}
	if rec.IsMeta || len(c.ToolResults()) > 0 {
		return
	}
	s.ImagesAttached += c.Images()
	if c.HasText() || c.Images() > 0 {
		// An unanswered prompt, such as one the user interrupted, is
		// dropped rather than merged into the next.
		sum.open = &openRequest{start: rec.Timestamp, messages: map[string]bool{}}
	}
}
