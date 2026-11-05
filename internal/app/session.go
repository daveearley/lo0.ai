package app

import (
	"context"
	"errors"
	"time"

	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/usage"
)

// RecentHistory reads the sessions a harness wrote since a time.
type RecentHistory interface {
	Since(ctx context.Context, t time.Time) ([]usage.Session, error)
}

// SessionReport is what one session started by a try did, read back from
// the harness's own logs.
type SessionReport struct {
	Harness string `json:"harness"`
	Model   string `json:"model"`
	Runtime string `json:"runtime"`
	// Window is the context window the model ran with.
	Window   int       `json:"window"`
	Started  time.Time `json:"started"`
	Ended    time.Time `json:"ended"`
	ExitCode int       `json:"exitCode"`
	// Found is true when lo0 found the session in the harness's logs; the
	// fields below are zero without it.
	Found       bool `json:"found"`
	Requests    int  `json:"requests"`
	PeakContext int  `json:"peakContext"`
	Compactions int  `json:"compactions"`
	ToolResults int  `json:"toolResults"`
	ToolErrors  int  `json:"toolErrors"`
	// UsualToolErrors is the share of tool calls that fail in the rest of
	// the user's history, to compare with.
	UsualToolErrors float64 `json:"usualToolErrors"`
	// Speed is the model's measured median speed and Replies the replies it
	// was measured from; zero when no reply was long enough to time.
	Speed   float64 `json:"speed,omitzero"`
	Replies int     `json:"replies,omitzero"`
	// Estimate is the speed lo0 expected, and Pace the measured speed
	// against the user's reference, the first of References.
	Estimate   float64           `json:"estimate,omitzero"`
	Pace       match.Pace        `json:"pace"`
	References []match.Reference `json:"references,omitzero"`
}

// SessionReport reads back the session plan p started at started, which
// ended at ended with exit code exit. It finds the session in the harness's
// logs by the model's name, which only lo0 uses, so a session with any
// other model is never mistaken for it.
func (s *Service) SessionReport(ctx context.Context, a Assessment, p migrate.Plan, started, ended time.Time, exit int) (SessionReport, error) {
	r := SessionReport{
		Harness: p.HarnessName, Model: p.Model, Runtime: p.RuntimeName, Window: p.Context,
		Started: started, Ended: ended, ExitCode: exit,
		UsualToolErrors: a.Scan.Usage.ToolErrors.Share,
		References:      a.Match.References,
	}
	for _, c := range a.Match.Candidates {
		if c.Model.Name == p.Model {
			r.Estimate = c.Best.TokensPerSecond
		}
	}
	var found []usage.Session
	var errs []error
	for _, h := range s.migrator.Histories {
		sessions, err := h.Since(ctx, started)
		if ctx.Err() != nil {
			return r, ctx.Err()
		}
		errs = append(errs, err)
		for _, sess := range sessions {
			if sess.Models[p.Served.Model] > 0 && !sess.End.Before(started) {
				found = append(found, sess)
			}
		}
	}
	for _, sess := range found {
		r.Found = true
		r.Requests += len(sess.Requests)
		r.PeakContext = max(r.PeakContext, sess.PeakContext)
		r.Compactions += sess.Compactions
		r.ToolResults += sess.ToolResults
		r.ToolErrors += sess.ToolErrors
	}
	for _, sp := range usage.Aggregate(found, 0, noRepos{}).Speeds {
		if sp.Model == p.Served.Model {
			r.Speed, r.Replies = sp.TokensPerSecond, sp.Replies
			r.Pace = match.PaceOf(sp.TokensPerSecond, 0, a.Match.References)
		}
	}
	return r, errors.Join(errs...)
}

// noRepos resolves no repositories: a session report needs none.
type noRepos struct{}

func (noRepos) Repo(string) string { return "" }
