package app

import (
	"cmp"
	"strconv"
	"strings"
	"time"

	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/runtimes"
)

// StepKind is what a next step does.
type StepKind string

// Next steps.
const (
	StepTry     StepKind = "try"
	StepKeep    StepKind = "keep"
	StepInstall StepKind = "install"
	StepCompare StepKind = "compare"
	StepRemove  StepKind = "remove"
	StepBrowse  StepKind = "browse"
)

// NextStep is one thing lo0 suggests doing next.
type NextStep struct {
	Kind  StepKind `json:"kind"`
	Label string   `json:"label"`
	// Note says what the step costs or why it is worth doing.
	Note    string      `json:"note,omitzero"`
	Model   string      `json:"model,omitzero"`
	Harness harness.ID  `json:"harness,omitzero"`
	Runtime runtimes.ID `json:"runtime,omitzero"`
	// Remove is the runtime's name for a model to remove.
	Remove string `json:"remove,omitzero"`
}

// Overview is the state of the user's local setup and what to do next,
// best step first.
type Overview struct {
	// Headline says in a sentence or two where things stand. Status is
	// its first sentence alone, for a front end that draws the speed.
	Headline string     `json:"headline"`
	Status   string     `json:"status,omitzero"`
	Steps    []NextStep `json:"steps"`
	// Kept is the local model a harness is set up to use, if any.
	Kept string `json:"kept,omitzero"`
	// LastTry is the newest session that used a model lo0 set up, and
	// LastTryModel that model's name.
	LastTry      time.Time `json:"lastTry,omitzero"`
	LastTryModel string    `json:"lastTryModel,omitzero"`
}

// Next decides what the user should do next with their local setup, from
// the assessment, what is installed and the harnesses. It is a function of
// what it is given, so every front end reaches the same steps.
func Next(a Assessment, inv Inventory, views []HarnessView) Overview {
	var o Overview
	o.LastTry, o.LastTryModel = lastTry(a)
	best, ok := a.Match.Best()
	if !ok {
		o.Headline = a.Match.Verdict.Headline
		o.Steps = []NextStep{{Kind: StepBrowse, Label: "See what it would take: every model and what it needs"}}
		return o
	}
	target := recommendedView(views, a.Match.TargetID)
	where := cmp.Or(target.Product, target.Name)
	if u, ok := keptUse(views, best); ok {
		o.Kept = u.Model
	}

	// With no history every share is zero, so the best model is named as the
	// best for the machine and the verdict says why there is no share.
	noHistory := a.Match.Verdict.Level == match.LevelNoHistory
	fits := "fits " + format.Percent(best.Best.Coverage) + " of your sessions"
	if noHistory {
		fits = "is the best fit for this machine"
	}
	var head []string
	switch {
	case o.Kept != "" && noHistory:
		head = append(head, best.Model.Name+" is set up as your local model.")
	case o.Kept != "":
		head = append(head, best.Model.Name+" is set up as your local model, and "+fits+".")
	case best.Installed != nil:
		head = append(head, best.Model.Name+" is already downloaded and "+fits+".")
	default:
		head = append(head, best.Model.Name+" "+fits+"; it is a "+format.Bytes(uint64(best.Best.Quant.Bytes))+" download.")
	}
	switch {
	case noHistory:
		head = append(head, a.Match.Verdict.Headline)
	case len(a.Match.References) > 0:
		head = append(head, match.PaceSentence(best.Pace, a.Match.References[0]))
	}
	o.Headline, o.Status = strings.Join(head, " "), head[0]

	tried := o.LastTryModel == best.Model.Name
	try := NextStep{
		Kind: StepTry, Label: "Try " + best.Model.Name + " in " + where + " for one session", Note: "nothing in " + where + " changes",
		Model: best.Model.ID, Harness: target.ID,
	}
	if best.Installed == nil {
		try.Note = "a " + format.Bytes(uint64(best.Best.Quant.Bytes)) + " download first"
	}
	keep := NextStep{
		Kind: StepKeep, Label: "Keep it: set it up in " + where + " for good", Note: "undo any time",
		Model: best.Model.ID, Harness: target.ID,
	}
	switch {
	case target.Checked && !target.Installation.Installed:
		o.Steps = append(o.Steps, NextStep{Kind: StepInstall, Label: "Install " + where, Note: "with its own installer", Harness: target.ID})
	case o.Kept != "":
	case tried:
		o.Steps = append(o.Steps, keep, try)
	default:
		o.Steps = append(o.Steps, try, keep)
	}

	// The model to compare with is the next best that lo0 is not about to
	// suggest removing.
	remove, removable := beaten(a, inv, best)
	for _, next := range a.Match.Candidates[1:] {
		if removable && next.Model.ID == remove.Model {
			continue
		}
		o.Steps = append(o.Steps, NextStep{
			Kind: StepCompare, Label: "Compare it with " + next.Model.Name,
			Note: strconv.Itoa(int(next.Best.TokensPerSecond)) + " tok/s, needs " + format.Bytes(uint64(next.Best.MemoryBytes)), Model: next.Model.ID,
		})
		break
	}
	if removable {
		o.Steps = append(o.Steps, remove)
	}
	o.Steps = append(o.Steps, NextStep{Kind: StepBrowse, Label: "Browse every model that fits, and why", Note: fitCount(a.Match)})
	return o
}

func recommendedView(views []HarnessView, id harness.ID) HarnessView {
	for _, v := range views {
		if v.ID == id {
			return v
		}
	}
	return HarnessView{ID: id}
}

// keptUse finds a harness command set up to use best on this machine.
func keptUse(views []HarnessView, best match.Candidate) (harness.Use, bool) {
	for _, v := range views {
		for _, u := range v.Installation.Uses {
			if u.Local && (strings.Contains(u.Model, best.Model.Name) || strings.Contains(u.Model, catalog.LocalPrefix+best.Model.ID+":")) {
				return u, true
			}
		}
	}
	return harness.Use{}, false
}

// lastTry is the newest session that used a model lo0 created, which only
// a try or a migration sets up, and that model's catalogue name.
func lastTry(a Assessment) (time.Time, string) {
	var at time.Time
	var name string
	for _, r := range a.Scan.Usage.Records {
		for _, u := range r.Usage {
			if !strings.HasPrefix(u.Model, catalog.LocalPrefix) || !r.Start.After(at) {
				continue
			}
			at, name = r.Start, u.Model
			id, _, _ := strings.Cut(strings.TrimPrefix(u.Model, catalog.LocalPrefix), ":")
			for _, c := range a.Match.Candidates {
				if c.Model.ID == id {
					name = c.Model.Name
				}
			}
		}
	}
	return at, name
}

// beaten suggests removing a downloaded model the best fit outdoes: it
// fits at least as many sessions and writes faster.
func beaten(a Assessment, inv Inventory, best match.Candidate) (NextStep, bool) {
	if best.Installed == nil {
		return NextStep{}, false
	}
	for _, rt := range inv.Runtimes {
		if !rt.Removable {
			continue
		}
		for _, m := range rt.Models {
			if m.Parent != "" || m.CatalogID == "" || m.CatalogID == best.Model.ID {
				continue
			}
			for _, c := range a.Match.Candidates {
				if c.Model.ID != m.CatalogID || c.Best.Coverage > best.Best.Coverage || c.Best.TokensPerSecond >= best.Best.TokensPerSecond {
					continue
				}
				why := best.Model.Name + " fits more of your sessions, faster"
				if c.Best.Coverage == best.Best.Coverage {
					why = best.Model.Name + " fits the same sessions faster"
				}
				return NextStep{
					Kind: StepRemove, Label: "Free " + format.Bytes(m.Bytes) + ": remove " + m.Name, Note: why,
					Model: m.CatalogID, Runtime: rt.ID, Remove: m.Name,
				}, true
			}
		}
	}
	return NextStep{}, false
}

// fitCount says how many models fit and how many of them are downloaded.
func fitCount(m match.Result) string {
	downloaded := 0
	for _, c := range m.Candidates {
		if c.Installed != nil {
			downloaded++
		}
	}
	n := format.Plural(len(m.Candidates), "model fits", "models fit")
	if downloaded == 0 {
		return n
	}
	return n + " · " + format.Int(downloaded) + " downloaded"
}
