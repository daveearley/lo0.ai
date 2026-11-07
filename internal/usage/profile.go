package usage

import (
	"cmp"
	"maps"
	"slices"
	"time"

	"lo0.ai/lo0/internal/stats"
)

// ContextWindows are the context sizes, in tokens, that the profile reports
// fit against. They are the windows local models commonly ship with.
var ContextWindows = []int{32 << 10, 64 << 10, 128 << 10, 256 << 10, 1 << 20}

// Profile summarises a user's history with one coding agent, or with
// several pooled together.
type Profile struct {
	// Sessions counts sessions with at least one model response.
	Sessions int `json:"sessions"`
	// EmptySessions counts transcripts with no model responses.
	EmptySessions int        `json:"emptySessions,omitzero"`
	First         time.Time  `json:"first,omitzero"`
	Last          time.Time  `json:"last,omitzero"`
	Turns         int        `json:"turns"`
	Context       Context    `json:"context"`
	Compaction    Compaction `json:"compaction"`
	Subagents     Subagents  `json:"subagents"`
	// Tools tallies the harness's own tools. MCP tools are grouped under
	// MCPServers.
	Tools        []Tally `json:"tools,omitzero"`
	MCPServers   []Tally `json:"mcpServers,omitzero"`
	Languages    []Tally `json:"languages,omitzero"`
	Models       []Tally `json:"models,omitzero"`
	Entrypoints  []Tally `json:"entrypoints,omitzero"`
	Repos        []Repo  `json:"repos,omitzero"`
	DecodeErrors int     `json:"decodeErrors,omitzero"`
	// Usage totals tokens by model across every session and subagent.
	Usage         []ModelUsage  `json:"usage,omitzero"`
	Requests      Requests      `json:"requests"`
	Interruptions Interruptions `json:"interruptions"`
	Reasoning     Reasoning     `json:"reasoning"`
	Images        Images        `json:"images"`
	Changes       Changes       `json:"changes"`
	ToolErrors    ToolErrors    `json:"toolErrors"`
	// Skills tallies skills loaded by name; HookRuns tallies hook runs by
	// event.
	Skills   []Tally `json:"skills,omitzero"`
	HookRuns []Tally `json:"hookRuns,omitzero"`
	Format   Format  `json:"format"`
	// ContextWindow is the largest context window the harness reported
	// working within, or 0 if it does not say. A harness that compacts at
	// its own window never shows a session larger than this.
	ContextWindow int `json:"contextWindow,omitzero"`
	// Plan is the subscription the latest session that names one reported.
	Plan string `json:"plan,omitzero"`
	// Speeds measures how fast each model wrote its replies, by harness,
	// the most used recently first.
	Speeds []Speed `json:"speeds,omitzero"`
	// Records has one entry per session with model responses, for analyses
	// that need sessions individually. Records hold no content.
	Records []SessionRecord `json:"records,omitzero"`
	// Harnesses holds each harness's own profile when several are pooled.
	// Context and fit come from the pool; cost, carry-over and versions
	// belong to one harness, so they read its profile through Of.
	Harnesses []HarnessProfile `json:"harnesses,omitzero"`
}

// HarnessProfile is one harness's share of a pooled profile.
type HarnessProfile struct {
	Name    string  `json:"name"`
	Profile Profile `json:"profile"`
}

// Of returns the profile of one harness. A profile that was not pooled is
// returned as it is, so code that reads one harness works the same whether
// or not others were scanned.
func (p Profile) Of(harness string) Profile {
	if len(p.Harnesses) == 0 {
		return p
	}
	for _, h := range p.Harnesses {
		if h.Name == harness {
			return h.Profile
		}
	}
	return Profile{}
}

// SessionRecord is the slim, content-free record of one session.
type SessionRecord struct {
	Start       time.Time    `json:"start,omitzero"`
	End         time.Time    `json:"end,omitzero"`
	PeakContext int          `json:"peakContext"`
	Repo        string       `json:"repo,omitzero"`
	Harness     string       `json:"harness,omitzero"`
	Usage       []ModelUsage `json:"usage,omitzero"`
}

// Context describes the peak context size of sessions, in tokens.
type Context struct {
	P50 int   `json:"p50"`
	P90 int   `json:"p90"`
	P99 int   `json:"p99"`
	Max int   `json:"max"`
	Fit []Fit `json:"fit"`
	// Peaks holds every session's peak context in ascending order, so that
	// fit can be computed for any context window.
	Peaks []int `json:"peaks,omitzero"`
}

// Within returns the share of sessions whose peak context is at most tokens.
func (c Context) Within(tokens int) float64 {
	n, _ := slices.BinarySearch(c.Peaks, tokens+1)
	return stats.Share(n, len(c.Peaks))
}

// Fit says how many sessions never exceeded a context window.
type Fit struct {
	Tokens   int     `json:"tokens"`
	Sessions int     `json:"sessions"`
	Share    float64 `json:"share"`
}

// Compaction describes how often sessions ran out of context.
type Compaction struct {
	Sessions int     `json:"sessions"`
	Share    float64 `json:"share"`
	Total    int     `json:"total"`
	Auto     int     `json:"auto"`
}

// Subagents describes subagent use.
type Subagents struct {
	Sessions    int     `json:"sessions"`
	Share       float64 `json:"share"`
	Calls       int     `json:"calls"`
	Transcripts int     `json:"transcripts"`
}

// Tally counts uses of one named thing and the sessions that used it.
type Tally struct {
	Name     string `json:"name"`
	Count    int    `json:"count"`
	Sessions int    `json:"sessions"`
	// Tools lists the distinct tools called, for MCP servers.
	Tools []string `json:"tools,omitzero"`
}

// Repo is the usage attributed to one repository.
type Repo struct {
	Path       string `json:"path"`
	Sessions   int    `json:"sessions"`
	Turns      int    `json:"turns"`
	ContextP90 int    `json:"contextP90"`
	ContextMax int    `json:"contextMax"`
}

// Progress receives the number of sessions read so far out of total. It is
// called from several goroutines at once and must be safe for that.
type Progress func(done, total int)

// RepoResolver maps a working directory to its repository root.
type RepoResolver interface {
	Repo(dir string) string
}

// Aggregate builds a Profile from session summaries. subagentTranscripts is
// the number of subagent transcript files found alongside them.
func Aggregate(sessions []Session, subagentTranscripts int, repos RepoResolver) Profile {
	p := Profile{Subagents: Subagents{Transcripts: subagentTranscripts}}
	var (
		peaks       []int
		tools       = tallies{}
		mcp         = tallies{}
		languages   = tallies{}
		models      = tallies{}
		entrypoints = tallies{}
		byRepo      = map[string]*repoAccumulator{}
		mcpTools    = map[string][]string{}
		w           = newWork()
		planAt      time.Time
	)
	for _, s := range sessions {
		p.DecodeErrors += s.DecodeErrors
		w.addFormat(s)
		if s.Turns == 0 {
			p.EmptySessions++
			continue
		}
		p.Sessions++
		p.Turns += s.Turns
		w.add(s)
		peaks = append(peaks, s.PeakContext)
		if !s.Start.IsZero() && (p.First.IsZero() || s.Start.Before(p.First)) {
			p.First = s.Start
		}
		if s.Plan != "" && !s.Start.Before(planAt) {
			p.Plan, planAt = s.Plan, s.Start
		}
		if s.End.After(p.Last) {
			p.Last = s.End
		}
		if s.Compactions > 0 {
			p.Compaction.Sessions++
			p.Compaction.Total += s.Compactions
			p.Compaction.Auto += s.AutoCompactions
		}
		if s.SubagentCalls > 0 {
			p.Subagents.Sessions++
			p.Subagents.Calls += s.SubagentCalls
		}
		tools.addAll(s.Tools)
		servers := map[string]int{}
		for call, n := range s.MCP {
			servers[call.Server] += n
			mcpTools[call.Server] = append(mcpTools[call.Server], call.Tool)
		}
		mcp.addAll(servers)
		languages.addAll(s.Languages)
		models.addAll(s.Models)
		if s.Entrypoint != "" {
			entrypoints.add(s.Entrypoint, 1)
		}
		p.Usage = MergeUsages(p.Usage, s.Usage)
		repo := repos.Repo(s.CWD)
		p.ContextWindow = max(p.ContextWindow, s.ContextWindow)
		p.Records = append(p.Records, SessionRecord{Start: s.Start, End: s.End, PeakContext: s.PeakContext, Repo: repo, Harness: s.Harness, Usage: s.Usage})
		if repo != "" {
			acc := byRepo[repo]
			if acc == nil {
				acc = &repoAccumulator{}
				byRepo[repo] = acc
			}
			acc.sessions++
			acc.turns += s.Turns
			acc.peaks = append(acc.peaks, s.PeakContext)
		}
	}

	p.Context = contextOf(peaks)
	p.Compaction.Share = stats.Share(p.Compaction.Sessions, p.Sessions)
	p.Subagents.Share = stats.Share(p.Subagents.Sessions, p.Sessions)
	p.Tools = tools.sorted()
	p.MCPServers = mcp.sorted()
	for i, t := range p.MCPServers {
		names := slices.Sorted(slices.Values(mcpTools[t.Name]))
		p.MCPServers[i].Tools = slices.Compact(names)
	}
	p.Languages = languages.sorted()
	p.Models = models.sorted()
	p.Entrypoints = entrypoints.sorted()
	p.Repos = reposOf(byRepo)
	p.Speeds = speedsOf(sessions)
	w.fill(&p)
	return p
}

func contextOf(peaks []int) Context {
	c := Context{
		Peaks: slices.Sorted(slices.Values(peaks)),
		P50:   stats.Percentile(peaks, 50),
		P90:   stats.Percentile(peaks, 90),
		P99:   stats.Percentile(peaks, 99),
		Max:   slices.Max(append([]int{0}, peaks...)),
	}
	for _, window := range ContextWindows {
		n := 0
		for _, peak := range peaks {
			if peak <= window {
				n++
			}
		}
		c.Fit = append(c.Fit, Fit{Tokens: window, Sessions: n, Share: stats.Share(n, len(peaks))})
	}
	return c
}

type repoAccumulator struct {
	sessions, turns int
	peaks           []int
}

func reposOf(byRepo map[string]*repoAccumulator) []Repo {
	repos := make([]Repo, 0, len(byRepo))
	for path, acc := range byRepo {
		repos = append(repos, Repo{
			Path:       path,
			Sessions:   acc.sessions,
			Turns:      acc.turns,
			ContextP90: stats.Percentile(acc.peaks, 90),
			ContextMax: slices.Max(acc.peaks),
		})
	}
	slices.SortFunc(repos, func(a, b Repo) int {
		return cmp.Or(cmp.Compare(b.Sessions, a.Sessions), cmp.Compare(a.Path, b.Path))
	})
	return repos
}

// tallies accumulates Tally values. Each call to add or addAll must cover one
// session, and must not repeat a name within it.
type tallies map[string]*Tally

func (t tallies) add(name string, n int) {
	e := t[name]
	if e == nil {
		e = &Tally{Name: name}
		t[name] = e
	}
	e.Count += n
	e.Sessions++
}

func (t tallies) addAll(counts map[string]int) {
	for name, n := range counts {
		t.add(name, n)
	}
}

func (t tallies) sorted() []Tally {
	out := make([]Tally, 0, len(t))
	for _, name := range slices.Sorted(maps.Keys(t)) {
		out = append(out, *t[name])
	}
	slices.SortStableFunc(out, func(a, b Tally) int { return cmp.Compare(b.Count, a.Count) })
	return out
}
