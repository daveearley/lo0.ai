package app

import (
	"fmt"

	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/runtimes"
)

// PlanRemove plans removing a model from a runtime. A download goes with
// the models made from it, since they share its weights and removing it
// alone would free nothing. The plan says how much space it frees and
// which of lo0's files use what it removes. It changes nothing.
func (s *Service) PlanRemove(a Assessment, rt runtimes.ID, model string) (migrate.Plan, error) {
	if err := onThisMachine(a); err != nil {
		return migrate.Plan{}, err
	}
	inv, err := s.Inventory(a)
	if err != nil {
		return migrate.Plan{}, err
	}
	var found *InstalledModel
	var made []InstalledModel
	for _, r := range inv.Runtimes {
		if r.ID != rt {
			continue
		}
		for i, m := range r.Models {
			if m.Name == model {
				found = &r.Models[i]
			}
			if m.Parent == model {
				made = append(made, m)
			}
		}
	}
	if found == nil {
		return migrate.Plan{}, fmt.Errorf("%s has no model named %s", rt.Name(), model)
	}
	b := s.remover(rt)
	if b == nil {
		return migrate.Plan{}, fmt.Errorf("lo0 cannot remove models from %s; remove %s there", rt.Name(), model)
	}
	var runtime runtimes.Runtime
	for _, r := range a.Scan.Runtimes {
		if r.ID == rt {
			runtime = r
		}
	}
	remove := func(m InstalledModel, note string) migrate.Step {
		step := b.Remove(runtime, m.Name)
		if note != "" {
			step.Detail = append(step.Detail, note)
		}
		for _, path := range m.UsedBy {
			step.Detail = append(step.Detail, path+" uses it and stops working until it is set up again.")
		}
		return step
	}
	var steps []migrate.Step
	for _, m := range made {
		steps = append(steps, remove(m, "Made from "+model+", whose weights it shares."))
	}
	note := "Frees " + format.Bytes(found.Bytes) + "."
	if found.Parent != "" {
		note = "Frees no space: its weights are " + found.Parent + "'s, which stays."
	}
	steps = append(steps, remove(*found, note))
	return migrate.Plan{
		Command: "remove", Runtime: rt, RuntimeName: rt.Name(), Model: model,
		Steps: steps,
	}, nil
}

// PlanPull plans downloading a catalogued model into a runtime and setting
// it up with the window lo0 recommends, ready to try, without starting any
// harness. It changes nothing.
func (s *Service) PlanPull(a Assessment, choice MigrationChoice) (migrate.Plan, error) {
	h, b, req, err := s.request(a, choice, false)
	if err != nil {
		return migrate.Plan{}, err
	}
	served, steps, err := b.Prepare(req, h.Protocol())
	if err != nil {
		return migrate.Plan{}, err
	}
	return migrate.Plan{
		Command: "pull", Runtime: served.Runtime, RuntimeName: served.RuntimeName,
		Model: req.Model.Name, Quant: req.Quant.Name, Context: req.Context, Served: served,
		Steps: steps,
		Then:  []string{"To try it, run lo0 try --model " + req.Model.ID + "."},
	}, nil
}
