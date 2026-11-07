// Package app holds lo0's use cases. Every front end, the command line, the
// terminal UI and the desktop app, calls this package and nothing below it,
// so they all reach the same answers.
package app

import (
	"context"
	"errors"
	"slices"

	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/claudecode"
	"lo0.ai/lo0/internal/codex"
	"lo0.ai/lo0/internal/cost"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/scan"
	"lo0.ai/lo0/internal/system"
	"lo0.ai/lo0/internal/usage"
)

// Scanner runs the read-only scan.
type Scanner interface {
	Run(ctx context.Context, observe scan.Observer) (scan.Result, error)
}

// Harness is a coding harness lo0 can assess a setup against, migrate it
// to and start for a single session. Each lives in its own package under
// internal/harness.
type Harness interface {
	match.Harness
	migrate.Harness
	migrate.Launcher
	// Status says whether the harness is installed and what it uses now.
	// It only reads.
	Status(ctx context.Context, req migrate.Request, files migrate.Files, runner system.Runner) harness.Installation
	// Install is the step that installs the harness with its own
	// installer.
	Install(req migrate.Request) migrate.Step
	// Product is the harness's own name, such as "Claude Code", for labels
	// where Name would be too long.
	Product() string
}

// Service runs lo0's use cases.
type Service struct {
	scanner   Scanner
	catalog   *catalog.Catalog
	harnesses []Harness
	migrator  Migrator
}

// New returns a Service. The scanner, the catalog and at least one harness
// are required; harnesses are listed in the order lo0 prefers them on a tie.
// A zero Migrator gives a service that assesses but cannot migrate.
func New(scanner Scanner, cat *catalog.Catalog, harnesses []Harness, mig Migrator) (*Service, error) {
	if scanner == nil || cat == nil || len(harnesses) == 0 {
		return nil, errors.New("app: scanner, catalog and harnesses are required")
	}
	return &Service{scanner: scanner, catalog: cat, harnesses: slices.Clone(harnesses), migrator: mig}, nil
}

// Scan profiles the machine, local runtimes, the Claude Code setup and every
// harness's history. observe, which may be nil, receives progress.
func (s *Service) Scan(ctx context.Context, observe scan.Observer) (scan.Result, error) {
	return s.scanner.Run(ctx, observe)
}

// Assessment is a scan together with what lo0 concluded from it.
type Assessment struct {
	Scan  scan.Result  `json:"scan"`
	Match match.Result `json:"match"`
	Cost  cost.Summary `json:"cost"`
}

// Assess scans, then matches the result against the model catalog and every
// harness, and prices each harness's history.
func (s *Service) Assess(ctx context.Context, observe scan.Observer) (Assessment, error) {
	res, err := s.Scan(ctx, observe)
	if err != nil {
		return Assessment{Scan: res}, err
	}
	// An unreadable benchmarks file degrades to estimates. Measurements from
	// this machine do not apply to simulated hardware.
	if !res.Machine.Simulated {
		res.Benchmarks, _ = s.benchmarks()
	}
	res.Harnesses = s.installations(ctx, res)
	mappers := make([]match.Harness, len(s.harnesses))
	for i, h := range s.harnesses {
		mappers[i] = h
	}
	m := match.Match(res, s.catalog, mappers)
	return Assessment{Scan: res, Match: m, Cost: Price(res, m)}, nil
}

// Price values each harness's history at API prices against the plan it
// runs on, and what the match's best model could take over. A profile that
// was not pooled is Claude Code's alone.
func Price(res scan.Result, m match.Result) cost.Summary {
	var local cost.Local
	if best, ok := m.Best(); ok {
		local = cost.Local{Model: best.Model.Name, Window: best.Best.MaxContext, TokensPerSecond: best.Best.TokensPerSecond}
	}
	var codexUse usage.Profile
	if len(res.Usage.Harnesses) > 0 {
		codexUse = res.Usage.Of(codex.Harness)
	}
	return cost.Summarize([]cost.Harness{
		{
			Name: claudecode.Harness, Profile: res.Usage.Of(claudecode.Harness),
			Plan: cost.PlanFor(res.Setup.Account), ExtraUsage: res.Setup.Account.ExtraUsage,
		},
		{Name: codex.Harness, Profile: codexUse, Plan: cost.ChatGPTPlanFor(codexUse.Plan)},
	}, local)
}

// Models lists the catalog, for front ends that let the user browse it.
func (s *Service) Models() []catalog.Model {
	return slices.Clone(s.catalog.Models)
}

// ProjectPaths lists the user's own project directories found in a scan,
// busiest first, for removing from anything shared.
func ProjectPaths(res scan.Result) []string {
	var out []string
	for _, r := range res.Usage.Repos {
		out = append(out, r.Path)
	}
	for _, rec := range res.Usage.Records {
		if rec.Repo != "" {
			out = append(out, rec.Repo)
		}
	}
	out = append(out, res.Setup.Projects...)
	for _, f := range res.Setup.Instructions {
		out = append(out, f.Project)
	}
	for _, s := range res.Setup.MCPServers {
		out = append(out, s.Project)
	}
	for _, h := range res.Setup.Hooks {
		out = append(out, h.Project)
	}
	for _, e := range res.Setup.Extensions {
		out = append(out, e.Project)
	}
	for _, p := range res.Setup.Permissions {
		out = append(out, p.Project)
	}
	for _, s := range res.Setup.Settings {
		out = append(out, s.Project)
	}
	seen := map[string]bool{}
	return slices.DeleteFunc(out, func(p string) bool {
		drop := p == "" || seen[p]
		seen[p] = true
		return drop
	})
}
