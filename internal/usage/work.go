package usage

import (
	"slices"
	"time"

	"lo0.ai/lo0/internal/stats"
	"lo0.ai/lo0/internal/version"
)

// Spread summarises a distribution by its median, 90th percentile and
// maximum.
type Spread struct {
	P50 int `json:"p50"`
	P90 int `json:"p90"`
	Max int `json:"max"`
}

func spreadOf(values []int) Spread {
	if len(values) == 0 {
		return Spread{}
	}
	return Spread{P50: stats.Percentile(values, 50), P90: stats.Percentile(values, 90), Max: slices.Max(values)}
}

// Requests describes how a prompt unfolds into an answer: how many tool
// calls the model chains, how much it writes and how long the user waits.
// Long chains are where local models most often lose their way.
type Requests struct {
	Count     int    `json:"count"`
	ToolCalls Spread `json:"toolCalls"`
	// Output is tokens written per request.
	Output Spread `json:"output"`
	// Seconds is the wait from prompt to answer.
	Seconds Spread `json:"seconds"`
}

// Interruptions counts what stopped or slowed a hosted model: usage limits,
// failed responses and retries, many of them while offline. None of these
// happen to a model running on the same machine.
type Interruptions struct {
	LimitHits       int `json:"limitHits"`
	LimitSessions   int `json:"limitSessions"`
	FailedResponses int `json:"failedResponses"`
	Retries         int `json:"retries"`
	// NetworkRetries are retries after the connection failed or the
	// machine went offline.
	NetworkRetries int `json:"networkRetries"`
	// Sessions counts sessions hit by any of them.
	Sessions int `json:"sessions"`
}

// Reasoning describes how much the user leans on extended thinking.
type Reasoning struct {
	// Effort tallies turns by requested effort, such as "high".
	Effort         []Tally `json:"effort,omitzero"`
	ThinkingTokens int64   `json:"thinkingTokens"`
	// ThinkingShare is the share of output tokens spent thinking.
	ThinkingShare float64 `json:"thinkingShare"`
	// HighEffortShare is the share of turns at high effort or above.
	HighEffortShare float64 `json:"highEffortShare"`
}

// highEfforts are the effort levels above the default.
var highEfforts = map[string]bool{"high": true, "xhigh": true, "max": true}

// Images counts pictures in the history. A local model needs vision to use
// any of them.
type Images struct {
	Attached         int `json:"attached"`
	AttachedSessions int `json:"attachedSessions"`
	// FromTools are images tools returned, such as browser screenshots.
	FromTools    int `json:"fromTools"`
	ToolSessions int `json:"toolSessions"`
	// Sessions counts sessions with any image.
	Sessions int `json:"sessions"`
}

// Changes is what the sessions produced.
type Changes struct {
	LinesAdded   int `json:"linesAdded"`
	LinesRemoved int `json:"linesRemoved"`
	// Files counts distinct files changed.
	Files        int `json:"files"`
	PullRequests int `json:"pullRequests"`
	PRSessions   int `json:"prSessions"`
}

// ToolErrors is how often a tool call failed and the model had to recover.
type ToolErrors struct {
	Results int     `json:"results"`
	Failed  int     `json:"failed"`
	Share   float64 `json:"share"`
}

// Format is evidence of how well lo0 understood a harness's logs, so a change
// in their format shows up instead of silently skewing the numbers.
type Format struct {
	// Responses counts model responses; WithoutUsage those with no token
	// counts.
	Responses    int `json:"responses"`
	WithoutUsage int `json:"withoutUsage,omitzero"`
	// UnknownTypes tallies record types lo0 has not seen before.
	UnknownTypes []Tally `json:"unknownTypes,omitzero"`
	// Versions tallies sessions by the harness version that last wrote to
	// them, newest first.
	Versions []Tally `json:"versions,omitzero"`
}

// Newest is the newest harness version in the history.
func (f Format) Newest() string {
	if len(f.Versions) == 0 {
		return ""
	}
	return f.Versions[0].Name
}

// Oldest is the oldest harness version in the history.
func (f Format) Oldest() string {
	if len(f.Versions) == 0 {
		return ""
	}
	return f.Versions[len(f.Versions)-1].Name
}

// work accumulates the per-request and per-session signals.
type work struct {
	toolCalls, output, seconds []int
	interruptions              Interruptions
	effort                     tallies
	thinking, outputTokens     int64
	images                     Images
	changes                    Changes
	files                      map[string]bool
	toolErrors                 ToolErrors
	skills, hooks              tallies
	format                     Format
	unknown, versions          tallies
}

func newWork() *work {
	return &work{effort: tallies{}, files: map[string]bool{}, skills: tallies{}, hooks: tallies{}, unknown: tallies{}, versions: tallies{}}
}

func (w *work) add(s Session) {
	for _, r := range s.Requests {
		w.toolCalls = append(w.toolCalls, r.ToolCalls)
		w.output = append(w.output, r.Output)
		if r.Duration > 0 {
			w.seconds = append(w.seconds, int(r.Duration/time.Second))
		}
	}
	in := &w.interruptions
	in.LimitHits += s.LimitHits
	in.FailedResponses += s.FailedResponses
	in.Retries += s.Retries
	in.NetworkRetries += s.NetworkRetries
	if s.LimitHits > 0 {
		in.LimitSessions++
	}
	if s.LimitHits+s.FailedResponses+s.Retries > 0 {
		in.Sessions++
	}
	w.effort.addAll(s.Effort)
	w.thinking += s.ThinkingTokens
	w.outputTokens += s.OutputTokens

	w.images.Attached += s.ImagesAttached
	w.images.FromTools += s.ToolImages
	if s.ImagesAttached > 0 {
		w.images.AttachedSessions++
	}
	if s.ToolImages > 0 {
		w.images.ToolSessions++
	}
	if s.ImagesAttached+s.ToolImages > 0 {
		w.images.Sessions++
	}

	w.changes.LinesAdded += s.LinesAdded
	w.changes.LinesRemoved += s.LinesRemoved
	for f := range s.Files {
		w.files[f] = true
	}
	w.changes.PullRequests += len(s.PullRequests)
	if len(s.PullRequests) > 0 {
		w.changes.PRSessions++
	}

	w.toolErrors.Results += s.ToolResults
	w.toolErrors.Failed += s.ToolErrors
	w.skills.addAll(s.Skills)
	w.hooks.addAll(s.Hooks)
}

// addFormat records how a transcript was written. It covers sessions with no
// usable turns too, since a format change can make every session look empty.
func (w *work) addFormat(s Session) {
	w.format.Responses += s.Responses
	w.format.WithoutUsage += s.WithoutUsage
	w.unknown.addAll(s.UnknownTypes)
	if s.Version != "" {
		w.versions.add(s.Version, 1)
	}
}

func (w *work) fill(p *Profile) {
	p.Requests = Requests{Count: len(w.toolCalls), ToolCalls: spreadOf(w.toolCalls), Output: spreadOf(w.output), Seconds: spreadOf(w.seconds)}
	p.Interruptions = w.interruptions

	p.Reasoning = Reasoning{Effort: w.effort.sorted(), ThinkingTokens: w.thinking}
	if w.outputTokens > 0 {
		p.Reasoning.ThinkingShare = float64(w.thinking) / float64(w.outputTokens)
	}
	high, all := 0, 0
	for _, t := range p.Reasoning.Effort {
		all += t.Count
		if highEfforts[t.Name] {
			high += t.Count
		}
	}
	p.Reasoning.HighEffortShare = stats.Share(high, all)

	p.Images = w.images
	p.Changes = w.changes
	p.Changes.Files = len(w.files)
	p.ToolErrors = w.toolErrors
	p.ToolErrors.Share = stats.Share(w.toolErrors.Failed, w.toolErrors.Results)
	p.Skills = w.skills.sorted()
	p.HookRuns = w.hooks.sorted()

	p.Format = w.format
	p.Format.UnknownTypes = w.unknown.sorted()
	versions := w.versions.sorted()
	slices.SortStableFunc(versions, func(a, b Tally) int { return version.Compare(b.Name, a.Name) })
	p.Format.Versions = versions
}
