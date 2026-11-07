package app

import (
	"context"
	"fmt"
	"slices"

	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/scan"
	"lo0.ai/lo0/internal/system"
)

// HarnessView is one harness as the user chooses between them: whether it
// is installed, what it uses now and how much of their setup carries over.
type HarnessView struct {
	ID   harness.ID `json:"id"`
	Name string     `json:"name"`
	// Product is the harness's own name, for short labels.
	Product      string               `json:"product"`
	Installation harness.Installation `json:"installation"`
	Fit          match.TargetFit      `json:"fit"`
	// Recommended is true for the harness most of the setup carries over to.
	Recommended bool `json:"recommended,omitzero"`
	// Checked is true when Installation was read from the machine; without
	// it, whether the harness is installed is not known.
	Checked bool `json:"checked"`
}

// Harnesses describes every harness lo0 can set up, in lo0's order of
// preference. It only reads: each harness looks at its own files and asks
// its program for its version.
func (s *Service) Harnesses(ctx context.Context, a Assessment) []HarnessView {
	req := s.baseRequest(a.Scan)
	runner := s.runner()
	out := make([]HarnessView, 0, len(s.harnesses))
	for _, h := range s.harnesses {
		fit, _ := a.Match.TargetByID(h.ID())
		out = append(out, HarnessView{
			ID: h.ID(), Name: h.Name(), Product: h.Product(), Fit: fit, Recommended: h.ID() == a.Match.TargetID, Checked: true,
			Installation: h.Status(ctx, req, s.files(), runner),
		})
	}
	return out
}

// PlanInstall plans installing a harness with its own installer. It
// changes nothing.
func (s *Service) PlanInstall(a Assessment, id harness.ID) (migrate.Plan, error) {
	if err := onThisMachine(a); err != nil {
		return migrate.Plan{}, err
	}
	i := slices.IndexFunc(s.harnesses, func(h Harness) bool { return h.ID() == id })
	if i < 0 {
		return migrate.Plan{}, fmt.Errorf("lo0 cannot install %q", id)
	}
	h := s.harnesses[i]
	return migrate.Plan{
		Command: "install", Harness: id, HarnessName: h.Name(),
		Steps: []migrate.Step{h.Install(s.baseRequest(a.Scan))},
	}, nil
}

// installations asks every harness how it is installed, for mapping the
// setup to the release that is there. It only reads.
func (s *Service) installations(ctx context.Context, res scan.Result) map[harness.ID]harness.Installation {
	req := s.baseRequest(res)
	out := make(map[harness.ID]harness.Installation, len(s.harnesses))
	for _, h := range s.harnesses {
		out[h.ID()] = h.Status(ctx, req, s.files(), s.runner())
	}
	return out
}

// baseRequest is what a harness reads its own state from.
func (s *Service) baseRequest(res scan.Result) migrate.Request {
	return migrate.Request{
		GOOS: s.migrator.GOOS, Home: s.migrator.Home, Getenv: s.migrator.Getenv, LookPath: s.migrator.LookPath,
		Layout: res.Layout, Setup: res.Setup,
	}
}

func (s *Service) runner() system.Runner {
	if s.migrator.Effects.Runner == nil {
		return noRunner{}
	}
	return s.migrator.Effects.Runner
}

// noRunner runs nothing, for a service built without a migrator.
type noRunner struct{}

func (noRunner) Run(context.Context, string, ...string) ([]byte, error) {
	return nil, system.ErrNotFound
}

func (noRunner) LookPath(name string) (string, error) {
	return "", fmt.Errorf("%s: %w", name, system.ErrNotFound)
}
