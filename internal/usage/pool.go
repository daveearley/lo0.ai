package usage

import (
	"context"
	"errors"
	"sync"
)

// Sessions is everything one harness's history yields: a summary per
// session, and how many subagent logs were folded into them.
type Sessions struct {
	List      []Session
	Subagents int
}

// Source reads one harness's history.
type Source interface {
	// Harness names the coding agent, as the report shows it.
	Harness() string
	// Sessions summarises every session. Logs that cannot be read are
	// skipped and reported in the error alongside the rest.
	Sessions(ctx context.Context, progress Progress) (Sessions, error)
}

// Pool reads several harnesses' histories and profiles them together. A
// session is a session whichever agent ran it, so context sizes, fit and the
// shape of the work come from every source at once; each harness's own
// profile is kept alongside for what differs between them.
type Pool struct {
	Sources []Source
	Repos   RepoResolver
}

// Analyze reads every source concurrently. progress receives the sum across
// sources. A source that fails partway contributes what it read, and its
// error is returned with the profile. Cancelling ctx returns ctx's error.
func (p Pool) Analyze(ctx context.Context, progress Progress) (Profile, error) {
	results := make([]Sessions, len(p.Sources))
	errs := make([]error, len(p.Sources))
	report := combined(len(p.Sources), progress)
	var wg sync.WaitGroup
	for i, src := range p.Sources {
		wg.Go(func() {
			results[i], errs[i] = src.Sessions(ctx, func(done, total int) { report(i, done, total) })
		})
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return Profile{}, err
	}

	var all []Session
	subagents := 0
	harnesses := make([]HarnessProfile, 0, len(p.Sources))
	for i, src := range p.Sources {
		name := src.Harness()
		for j := range results[i].List {
			results[i].List[j].Harness = name
		}
		all = append(all, results[i].List...)
		subagents += results[i].Subagents
		harnesses = append(harnesses, HarnessProfile{Name: name, Profile: Aggregate(results[i].List, results[i].Subagents, p.Repos)})
	}
	pooled := Aggregate(all, subagents, p.Repos)
	// Version numbers only compare within one harness, so the pooled
	// profile has none; each harness's profile keeps its own.
	pooled.Format.Versions = nil
	pooled.Harnesses = harnesses
	return pooled, errors.Join(errs...)
}

// combined turns per-source progress into one count. It is safe to call
// from several goroutines.
func combined(n int, progress Progress) func(source, done, total int) {
	if progress == nil {
		return func(int, int, int) {}
	}
	var mu sync.Mutex
	done, total := make([]int, n), make([]int, n)
	return func(source, d, t int) {
		mu.Lock()
		defer mu.Unlock()
		done[source], total[source] = d, t
		sumDone, sumTotal := 0, 0
		for i := range n {
			sumDone += done[i]
			sumTotal += total[i]
		}
		progress(sumDone, sumTotal)
	}
}
