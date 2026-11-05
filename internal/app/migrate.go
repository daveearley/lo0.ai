package app

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/runtimes"
)

// Migrator is what the service needs to plan and apply migrations on this
// machine.
type Migrator struct {
	// Backends are the runtimes lo0 can prepare a model in, in the order it
	// prefers them when the user has none running.
	Backends []migrate.Backend
	GOOS     string
	Home     string
	Getenv   func(string) string
	LookPath func(string) (string, error)
	Effects  migrate.Effects
	// Histories read back the sessions a try started.
	Histories []RecentHistory
	// Benchmarks is the file lo0 keeps its measurements in, and Swap reads
	// swap activity so a measurement under memory pressure is refused.
	Benchmarks string
	Swap       SwapMeter
}

// MigrationChoice says what to move to. Empty fields take lo0's
// recommendation: the harness most of the setup carries over to, the best
// model and the runtime the user already uses.
type MigrationChoice struct {
	Harness harness.ID
	Runtime runtimes.ID
	// Model is a catalogue model ID.
	Model string
}

// ErrNoFit means no catalogued model fits the machine, so there is nothing
// to migrate to.
var ErrNoFit = errors.New("no catalogued model fits this machine with a useful context window")

// PlanMigration plans the move described by choice for the assessed setup.
// It changes nothing. Files already as the plan would write them are
// marked done, so planning a move that is already made shows nothing to do.
func (s *Service) PlanMigration(a Assessment, choice MigrationChoice) (migrate.Plan, error) {
	h, b, req, err := s.request(a, choice, true)
	if err != nil {
		return migrate.Plan{}, err
	}
	p, err := migrate.Build(req, b, h)
	if err != nil {
		return migrate.Plan{}, err
	}
	return migrate.Settle(p, s.migrator.Effects), nil
}

// PlanTry plans one session of the harness described by choice with its
// model: the runtime is prepared as for a migration, but the harness's
// configuration is left as it is and everything it needs is passed to the
// session. It changes nothing.
func (s *Service) PlanTry(a Assessment, choice MigrationChoice) (migrate.Plan, error) {
	h, b, req, err := s.request(a, choice, true)
	if err != nil {
		return migrate.Plan{}, err
	}
	return migrate.BuildTry(req, b, h)
}

// request resolves choice against the assessment into the harness, the
// backend and what they plan from. With background, a harness that uses a
// background model gets the one lo0 picks to run alongside the chosen
// model.
func (s *Service) request(a Assessment, choice MigrationChoice, background bool) (Harness, migrate.Backend, migrate.Request, error) {
	h, target, err := s.harness(a.Match, choice.Harness)
	if err != nil {
		return nil, nil, migrate.Request{}, err
	}
	c, err := candidate(a.Match, choice.Model)
	if err != nil {
		return nil, nil, migrate.Request{}, err
	}
	b, rt, err := s.backend(a, c, choice.Runtime)
	if err != nil {
		return nil, nil, migrate.Request{}, err
	}
	req := migrate.Request{
		GOOS: s.migrator.GOOS, Home: s.migrator.Home, Getenv: s.migrator.Getenv, LookPath: s.migrator.LookPath,
		Layout: a.Scan.Layout, Setup: a.Scan.Setup, Features: target.Features,
		Model: c.Model, Quant: c.Best.Quant, Context: c.Best.MaxContext, Runtime: rt,
		Target: a.Scan.Harnesses[h.ID()],
	}
	if c.UsesInstalled() && c.Installed.Runtime == rt.ID {
		req.Installed = c.Installed.Tag
	}
	if user, ok := h.(migrate.BackgroundUser); ok && background && user.UsesBackground() {
		if bg, ok := a.Match.BackgroundFor(c); ok {
			req.Background = &migrate.Background{Model: bg.Model, Quant: bg.Option.Quant, Context: bg.Option.MaxContext}
		}
	}
	return h, b, req, nil
}

// PlanUndo plans reversing everything lo0 has changed, newest first. It
// only reads.
func (s *Service) PlanUndo() (migrate.Plan, error) {
	if len(s.migrator.Backends) == 0 {
		return migrate.Plan{}, errors.New("app: this service was built without a migrator")
	}
	return migrate.PlanUndo(s.migrator.Effects.Journal, s.migrator.Effects.Files, s.migrator.Backends)
}

// Migrate applies a plan from any of the Plan methods. observe, which may be
// nil, receives progress.
func (s *Service) Migrate(ctx context.Context, p migrate.Plan, observe migrate.Observer) (migrate.Outcome, error) {
	if len(s.migrator.Backends) == 0 {
		return migrate.Outcome{}, errors.New("app: this service was built without a migrator")
	}
	return migrate.Apply(ctx, p, s.migrator.Effects, observe)
}

func (s *Service) harness(m match.Result, id harness.ID) (Harness, match.TargetFit, error) {
	if id == "" {
		id = m.TargetID
	}
	target, ok := m.TargetByID(id)
	i := slices.IndexFunc(s.harnesses, func(h Harness) bool { return h.ID() == id })
	if !ok || i < 0 {
		return nil, match.TargetFit{}, fmt.Errorf("lo0 cannot set up %q", id)
	}
	return s.harnesses[i], target, nil
}

func candidate(m match.Result, id string) (match.Candidate, error) {
	if id == "" {
		if best, ok := m.Best(); ok {
			return best, nil
		}
		return match.Candidate{}, ErrNoFit
	}
	for _, c := range m.Candidates {
		if c.Model.ID == id {
			return c, nil
		}
	}
	return match.Candidate{}, fmt.Errorf("%s does not fit this machine with a useful context window, or is not in the catalogue", id)
}

// backend picks the runtime: the one asked for, else the one that already
// has the model, else the first running, then the first installed, then the
// first lo0 supports.
func (s *Service) backend(a Assessment, c match.Candidate, id runtimes.ID) (migrate.Backend, runtimes.Runtime, error) {
	found := func(id runtimes.ID) runtimes.Runtime {
		for _, rt := range a.Scan.Runtimes {
			if rt.ID == id {
				return rt
			}
		}
		return runtimes.Runtime{ID: id, Name: id.Name()}
	}
	if id == "" && c.Installed != nil {
		id = c.Installed.Runtime
	}
	for _, want := range []func(runtimes.Runtime) bool{
		func(rt runtimes.Runtime) bool { return rt.Running },
		func(rt runtimes.Runtime) bool { return rt.Installed },
		func(runtimes.Runtime) bool { return true },
	} {
		if id != "" {
			break
		}
		for _, b := range s.migrator.Backends {
			if want(found(b.ID())) {
				id = b.ID()
				break
			}
		}
	}
	for _, b := range s.migrator.Backends {
		if b.ID() == id {
			return b, found(id), nil
		}
	}
	return nil, runtimes.Runtime{}, fmt.Errorf("lo0 cannot set up models in %q", id)
}
