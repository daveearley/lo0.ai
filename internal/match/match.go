package match

import (
	"cmp"
	"math"
	"slices"
	"strings"

	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/claudecode"
	"lo0.ai/lo0/internal/claudecode/setup"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/machine"
	"lo0.ai/lo0/internal/runtimes"
	"lo0.ai/lo0/internal/scan"
	"lo0.ai/lo0/internal/stats"
	"lo0.ai/lo0/internal/usage"
)

// Harness is a coding harness lo0 can map a Claude Code setup to.
// Features maps the setup to the harness as it is installed, since what a
// harness reads can change between its releases; a zero Installation means
// it is not installed and the mapping is for the release lo0 would install.
type Harness interface {
	ID() harness.ID
	Name() string
	Features(inv setup.Inventory, history usage.Profile, installed harness.Installation) []harness.Feature
}

// TargetFit is how a setup carries over to one harness.
type TargetFit struct {
	ID       harness.ID        `json:"id"`
	Name     string            `json:"name"`
	Features []harness.Feature `json:"features"`
	// Score is the share of features that carry over in some form,
	// discounted by the share of MCP calls that went to tools that would be
	// lost.
	Score float64 `json:"score"`
}

// Result is the outcome of matching a scan against the catalog.
type Result struct {
	// Target and TargetID name the recommended harness, the one the most of
	// the setup carries over to; Features is its mapping.
	Target   string              `json:"target"`
	TargetID harness.ID          `json:"targetId"`
	Budget   machine.ModelMemory `json:"budget"`
	// Bandwidth is the memory bandwidth speed estimates assume.
	Bandwidth Bandwidth `json:"bandwidth"`
	// ReferenceContext is the context size speed is estimated at: halfway
	// through the median session, since context grows as a session runs.
	ReferenceContext int `json:"referenceContext"`
	// SpeedSource is the best evidence behind estimates for models not
	// measured here: this machine's calibration, published results for its
	// chip, or its bandwidth alone.
	SpeedSource Source `json:"speedSource"`
	// SpeedBasis names the published result estimates come from, with
	// where and when it was copied, when there is one.
	SpeedBasis string `json:"speedBasis,omitzero"`
	// References are the speeds the user is used to, measured from their
	// history, one per harness; candidates' pace is against the first.
	References []Reference `json:"references,omitzero"`
	// Candidates are the models that fit with useful context, best first.
	Candidates []Candidate `json:"candidates"`
	// Background is the model to run alongside the best candidate for a
	// harness's background work. Nil when none fits next to it, and
	// background work then uses the best candidate too.
	Background *Background `json:"background,omitzero"`
	TooLarge   []TooLarge  `json:"tooLarge,omitzero"`
	// Uncatalogued are installed models lo0 has no architecture data for.
	Uncatalogued []runtimes.Model  `json:"uncatalogued,omitzero"`
	Features     []harness.Feature `json:"features"`
	// Targets maps the setup to every harness, in the order lo0 prefers
	// them when they tie.
	Targets []TargetFit `json:"targets"`
	// Projects and Months measure the history against the best model's
	// window, by repository and by month. Both are empty when nothing fits.
	Projects []ProjectFit `json:"projects,omitzero"`
	Months   []MonthFit   `json:"months,omitzero"`
	Verdict  Verdict      `json:"verdict"`
	// CatalogDate is when the catalog was generated.
	CatalogDate string `json:"catalogDate"`

	// speeds and kvTypes are what options were evaluated with, so later
	// evaluations, such as a background model's, use the same evidence.
	speeds  speeds
	kvTypes []catalog.KVType
}

// UsesInstalled reports whether the recommendation is the build the user
// already has.
func (c Candidate) UsesInstalled() bool {
	return c.Installed != nil && c.Installed.Option.Quant == c.Best.Quant
}

// Best returns the top candidate, if any model fits.
func (r Result) Best() (Candidate, bool) {
	if len(r.Candidates) == 0 {
		return Candidate{}, false
	}
	return r.Candidates[0], true
}

// tierWeight discounts models that are less reliable in agent loops.
var tierWeight = map[catalog.Tier]float64{catalog.TierA: 1, catalog.TierB: 0.75, catalog.TierC: 0.4}

const (
	// comfortableSpeed is the decode rate above which an agent loop stops
	// feeling slow, in tokens per second.
	comfortableSpeed = 25
	// installedPreference is how close an installed build must come to the
	// best option to be recommended instead, saving a download.
	installedPreference = 0.95
)

// DefaultReferenceContext is the context size speed is estimated at when
// there is no history to take a median from.
const DefaultReferenceContext = 16 << 10

// Match evaluates every catalogued model against the machine and history in
// res, maps the setup to each harness, recommends one and reaches a verdict.
func Match(res scan.Result, cat *catalog.Catalog, harnesses []Harness) Result {
	budget := res.Machine.ModelMemory()
	out := Result{
		Budget:           budget,
		Bandwidth:        MemoryBandwidth(res.Machine),
		ReferenceContext: cmp.Or(res.Usage.Context.P50/2, DefaultReferenceContext),
		CatalogDate:      cat.Generated,
	}
	f := fitter{
		budget:    int64(budget.Bytes),
		context:   res.Usage.Context,
		reference: out.ReferenceContext,
		speeds:    newSpeeds(res.Machine, out.Bandwidth, res.Benchmarks, cat),
		kvTypes:   cacheTypes(res.Runtimes),
	}
	out.speeds, out.kvTypes = f.speeds, f.kvTypes
	out.SpeedSource, out.SpeedBasis = f.speeds.rates.source, f.speeds.rates.basis
	if f.speeds.calibrated {
		out.SpeedSource = SourceMachine
	}
	installed := installedModels(res.Runtimes, cat)
	out.References = references(res.Usage)

	for _, m := range cat.Models {
		if m.Tier == catalog.TierD {
			continue
		}
		have, isInstalled := installed[m.ID]
		options := f.options(m)
		top, ok := best(options)
		if !ok && !isInstalled {
			out.TooLarge = append(out.TooLarge, TooLarge{Model: m, NeedsBytes: f.smallestNeed(m), NeedsContext: minUsefulContext})
			continue
		}
		c := Candidate{Model: m, Best: top, Options: options}
		if isInstalled {
			c.Installed = &Installed{Runtime: have.runtime, Tag: have.model.Name, Option: f.installedOption(m, have.model), Capabilities: have.model.Capabilities}
			if !ok || c.Installed.Option.Utility() >= installedPreference*top.Utility() {
				c.Best = c.Installed.Option
			}
		}
		if c.Best.MaxContext < minUsefulContext {
			out.TooLarge = append(out.TooLarge, TooLarge{Model: m, NeedsBytes: f.smallestNeed(m), NeedsContext: minUsefulContext})
			continue
		}
		c.Score = score(c)
		c.Pace = paceOf(c.Best.TokensPerSecond, c.Best.ReadTokensPerSecond, out.References)
		out.Candidates = append(out.Candidates, c)
	}
	slices.SortStableFunc(out.Candidates, func(a, b Candidate) int {
		return cmp.Or(cmp.Compare(b.Score, a.Score), cmp.Compare(a.Model.ID, b.Model.ID))
	})
	slices.SortStableFunc(out.TooLarge, func(a, b TooLarge) int { return cmp.Compare(a.NeedsBytes, b.NeedsBytes) })

	for _, rt := range res.Runtimes {
		for _, m := range rt.Models {
			if _, ok := cat.ByOllamaTag(m.Name); !ok || rt.ID != runtimes.Ollama {
				out.Uncatalogued = append(out.Uncatalogued, m)
			}
		}
	}
	insights(&out, res.Usage)
	if best, ok := out.Best(); ok {
		out.Projects = projectFits(res.Usage.Records, best.Best.MaxContext)
		out.Months = monthFits(res.Usage.Records, best.Best.MaxContext)
		if bg, ok := out.BackgroundFor(best); ok {
			out.Background = &bg
		}
	}
	out.Targets = targets(res, harnesses)
	if top, ok := recommended(out.Targets); ok {
		out.Target, out.TargetID, out.Features = top.Name, top.ID, top.Features
	}
	out.Verdict = decide(out, res)
	return out
}

// setupHistory is the history of the harness the setup inventory came from.
// Carry-over weighs that setup by how it was used, so other harnesses'
// sessions, pooled in for fit, must not count towards it.
func setupHistory(res scan.Result) usage.Profile {
	return res.Usage.Of(claudecode.Harness)
}

func targets(res scan.Result, harnesses []Harness) []TargetFit {
	mcpCalls := totalMCPCalls(res)
	out := make([]TargetFit, 0, len(harnesses))
	for _, h := range harnesses {
		features := h.Features(res.Setup, setupHistory(res), res.Harnesses[h.ID()])
		lost, _ := harness.AppCalls(features, harness.StatusLost)
		out = append(out, TargetFit{
			ID: h.ID(), Name: h.Name(), Features: features,
			Score: harness.Summarize(features, "").CarriedShare() * (1 - stats.Share(lost, mcpCalls)),
		})
	}
	return out
}

// recommended is the target with the highest score, the first on a tie.
func recommended(ts []TargetFit) (TargetFit, bool) {
	if len(ts) == 0 {
		return TargetFit{}, false
	}
	best := ts[0]
	for _, t := range ts[1:] {
		if t.Score > best.Score {
			best = t
		}
	}
	return best, true
}

// TargetByID returns the mapping for one harness.
func (r Result) TargetByID(id harness.ID) (TargetFit, bool) {
	for _, t := range r.Targets {
		if t.ID == id {
			return t, true
		}
	}
	return TargetFit{}, false
}

// TargetNamed finds a harness by its ID or a prefix of it, ignoring case,
// for front ends that let the user type one.
func (r Result) TargetNamed(name string) (TargetFit, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, t := range r.Targets {
		if name != "" && strings.HasPrefix(string(t.ID), name) {
			return t, true
		}
	}
	return TargetFit{}, false
}

// score ranks candidates by the utility of their best option, discounted by
// how reliably the model drives an agent loop and by its size.
func score(c Candidate) float64 {
	return c.Best.Utility() * tierWeight[c.Model.Tier] * sizeWeight(c.Model)
}

const (
	// fullSizeParams is the size from which a model is no longer
	// discounted for being small.
	fullSizeParams = 8e9
	// sizeExponent sets how steeply smaller models are discounted: a 4B
	// model keeps 81% of its score and a 2B model 66%.
	sizeExponent = 0.3
)

// sizeWeight discounts models under fullSizeParams. Tiers rate how a model
// behaves in an agent loop, not how much it knows, so without it a fast 2B
// model would outrank a 9B model of the same tier that also fits. Total
// weights are the measure, since a mixture of experts knows what all its
// experts know.
func sizeWeight(m catalog.Model) float64 {
	return math.Pow(min(float64(m.Params)/fullSizeParams, 1), sizeExponent)
}

type installedModel struct {
	runtime runtimes.ID
	model   runtimes.Model
}

func installedModels(rts []runtimes.Runtime, cat *catalog.Catalog) map[string]installedModel {
	out := map[string]installedModel{}
	for _, rt := range rts {
		if rt.ID != runtimes.Ollama {
			continue
		}
		for _, m := range rt.Models {
			cm, ok := cat.ByOllamaTag(m.Name)
			if !ok {
				continue
			}
			// A model lo0 created already has its context window set, so it
			// is the one to keep using.
			if have, seen := out[cm.ID]; seen && strings.HasPrefix(have.model.Name, catalog.LocalPrefix) {
				continue
			}
			out[cm.ID] = installedModel{runtime: rt.ID, model: m}
		}
	}
	return out
}

// cacheTypes are the cache precisions lo0 can plan for. Ollama sets the
// cache type for its whole server, not per model, and lo0 does not change
// it, so with Ollama, which lo0 prefers when it is there or nothing is,
// only the default 16-bit cache is planned.
func cacheTypes(rts []runtimes.Runtime) []catalog.KVType {
	other := false
	for _, rt := range rts {
		if rt.Installed && rt.ID == runtimes.Ollama {
			return []catalog.KVType{catalog.KVF16}
		}
		other = other || rt.Installed
	}
	if !other {
		return []catalog.KVType{catalog.KVF16}
	}
	return []catalog.KVType{catalog.KVF16, catalog.KVQ8}
}
