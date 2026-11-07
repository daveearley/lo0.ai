// Package history reads Claude Code's session transcripts and summarises
// each session for the usage profile. It is the only place that knows the
// transcript format beyond decoding it; the usage package above it knows no
// harness at all.
package history

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"lo0.ai/lo0/internal/claudecode"
	"lo0.ai/lo0/internal/claudecode/transcript"
	"lo0.ai/lo0/internal/usage"
)

// Analyzer reads every session under a projects directory.
type Analyzer struct {
	projects fs.FS
	workers  int
}

// NewAnalyzer returns an Analyzer over projects, the Claude Code projects
// directory, reading up to workers transcripts at once.
func NewAnalyzer(projects fs.FS, workers int) *Analyzer {
	return &Analyzer{projects: projects, workers: max(workers, 1)}
}

// Harness implements usage.Source.
func (*Analyzer) Harness() string { return claudecode.Harness }

// Sessions summarises every session, with its subagents' token usage merged
// in. Transcripts that cannot be read are skipped and reported in the
// returned error alongside the rest. Cancelling ctx stops the read and
// returns ctx's error.
func (a *Analyzer) Sessions(ctx context.Context, progress usage.Progress) (usage.Sessions, error) {
	sessions, listErr := transcript.Discover(a.projects)
	if errors.Is(listErr, fs.ErrNotExist) && len(sessions) == 0 {
		listErr = nil
	}
	if listErr != nil {
		listErr = fmt.Errorf("listing transcripts: %w", listErr)
	}

	total := len(sessions)
	if progress != nil {
		progress(0, total)
	}
	summaries := make([]usage.Session, total)
	ids := make([][]string, total)
	errs := make([]error, total)
	var done atomic.Int64

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(a.workers)
	subagents := 0
	for i, s := range sessions {
		subagents += len(s.Subagents)
		g.Go(func() error {
			if err := gctx.Err(); err != nil {
				return err
			}
			summaries[i], ids[i], errs[i] = a.summarizeWithSubagents(s, nil)
			if progress != nil {
				progress(int(done.Add(1)), total)
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return usage.Sessions{}, err
	}
	if err := ctx.Err(); err != nil {
		return usage.Sessions{}, err
	}
	a.billOnce(sessions, summaries, ids, errs)
	return usage.Sessions{List: summaries, Subagents: subagents}, errors.Join(listErr, errors.Join(errs...))
}

// billOnce makes each response count towards one session only. A resumed or
// forked session's transcript repeats responses from the one it came from,
// with the same IDs, so the oldest session that has a response bills it and
// the few later ones that repeat it are read again without it.
func (a *Analyzer) billOnce(sessions []transcript.Session, summaries []usage.Session, ids [][]string, errs []error) {
	order := make([]int, len(sessions))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(x, y int) int { return summaries[x].Start.Compare(summaries[y].Start) })
	claimed := map[string]bool{}
	for _, i := range order {
		if slices.ContainsFunc(ids[i], func(id string) bool { return claimed[id] }) {
			summaries[i], _, errs[i] = a.summarizeWithSubagents(sessions[i], func(id string) bool { return claimed[id] })
		}
		for _, id := range ids[i] {
			claimed[id] = true
		}
	}
}

func (a *Analyzer) summarize(path string, billedElsewhere func(string) bool) (usage.Session, []string, error) {
	f, err := a.projects.Open(path)
	if err != nil {
		return usage.Session{}, nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()
	s, ids, err := summarize(transcript.Records(f), billedElsewhere)
	if err != nil {
		return s, ids, fmt.Errorf("reading %s: %w", path, err)
	}
	return s, ids, nil
}

// summarizeWithSubagents summarises a session and adds its subagents' token
// usage, which is billed like the session's own.
func (a *Analyzer) summarizeWithSubagents(s transcript.Session, billedElsewhere func(string) bool) (usage.Session, []string, error) {
	summary, ids, err := a.summarize(s.Main, billedElsewhere)
	errs := make([]error, 0, 1+len(s.Subagents))
	errs = append(errs, err)
	for _, path := range s.Subagents {
		sub, subIDs, err := a.summarize(path, billedElsewhere)
		errs = append(errs, err)
		summary.Usage = usage.MergeUsages(summary.Usage, sub.Usage)
		ids = append(ids, subIDs...)
	}
	return summary, ids, errors.Join(errs...)
}

// Since summarises the sessions whose transcript changed at or after t, so
// a session that just ended can be read without reading every other one.
// Transcripts that cannot be read are skipped and reported in the error.
func (a *Analyzer) Since(ctx context.Context, t time.Time) ([]usage.Session, error) {
	sessions, err := transcript.Discover(a.projects)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("listing transcripts: %w", err)
	}
	var out []usage.Session
	var errs []error
	for _, s := range sessions {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		info, err := fs.Stat(a.projects, s.Main)
		if err != nil || info.ModTime().Before(t) {
			continue
		}
		summary, _, err := a.summarizeWithSubagents(s, nil)
		errs = append(errs, err)
		summary.Harness = claudecode.Harness
		out = append(out, summary)
	}
	return out, errors.Join(errs...)
}
