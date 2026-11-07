package history

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sync/atomic"

	"golang.org/x/sync/errgroup"

	"lo0.ai/lo0/internal/codex"
	"lo0.ai/lo0/internal/codex/rollout"
	"lo0.ai/lo0/internal/usage"
)

// Analyzer reads every session log under a Codex home directory, archived
// ones included.
type Analyzer struct {
	home    fs.FS
	workers int
}

// NewAnalyzer returns an Analyzer over home, the Codex home directory,
// reading up to workers logs at once.
func NewAnalyzer(home fs.FS, workers int) *Analyzer {
	return &Analyzer{home: home, workers: max(workers, 1)}
}

// Harness implements usage.Source.
func (*Analyzer) Harness() string { return codex.Harness }

// Sessions summarises every session. Logs that cannot be read are skipped
// and reported in the returned error alongside the rest. Subagent sessions
// are folded into the session that started them: their tokens count towards
// it, and they are not sessions of their own. Cancelling ctx stops the read
// and returns ctx's error.
func (a *Analyzer) Sessions(ctx context.Context, progress usage.Progress) (usage.Sessions, error) {
	paths, listErr := a.discover()
	total := len(paths)
	if progress != nil {
		progress(0, total)
	}
	summaries := make([]Summary, total)
	errs := make([]error, total)
	var done atomic.Int64

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(a.workers)
	for i, p := range paths {
		g.Go(func() error {
			if err := gctx.Err(); err != nil {
				return err
			}
			summaries[i], errs[i] = a.summarize(p)
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
	sessions, subagents := foldSubagents(summaries)
	return usage.Sessions{List: sessions, Subagents: subagents}, errors.Join(listErr, errors.Join(errs...))
}

func (a *Analyzer) discover() ([]string, error) {
	var paths []string
	var errs []error
	for _, dir := range []string{codex.SessionsSubdir, codex.ArchivedSubdir} {
		sub, err := fs.Sub(a.home, dir)
		if err != nil {
			errs = append(errs, fmt.Errorf("listing Codex %s: %w", dir, err))
			continue
		}
		found, err := rollout.Discover(sub)
		if err != nil && (len(found) > 0 || !errors.Is(err, fs.ErrNotExist)) {
			errs = append(errs, fmt.Errorf("listing Codex %s: %w", dir, err))
		}
		for _, p := range found {
			paths = append(paths, path.Join(dir, p))
		}
	}
	return paths, errors.Join(errs...)
}

func (a *Analyzer) summarize(path string) (Summary, error) {
	f, err := a.home.Open(path)
	if err != nil {
		return Summary{}, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()
	s, err := Summarize(rollout.Records(f))
	if err != nil {
		return s, fmt.Errorf("reading %s: %w", path, err)
	}
	return s, nil
}

// foldSubagents merges each subagent's token usage into its parent and
// returns the sessions that remain, with how many subagent logs were found.
// A subagent whose parent's log is missing is dropped, as Claude Code's are.
func foldSubagents(summaries []Summary) ([]usage.Session, int) {
	byID := map[string]int{}
	for i, s := range summaries {
		if s.Parent == "" && s.Session.ID != "" {
			byID[s.Session.ID] = i
		}
	}
	subagents := 0
	for _, s := range summaries {
		if s.Parent == "" {
			continue
		}
		subagents++
		if i, ok := byID[s.Parent]; ok {
			parent := &summaries[i].Session
			parent.Usage = usage.MergeUsages(parent.Usage, s.Session.Usage)
		}
	}
	out := make([]usage.Session, 0, len(summaries)-subagents)
	for _, s := range summaries {
		if s.Parent == "" {
			out = append(out, s.Session)
		}
	}
	return out, subagents
}
