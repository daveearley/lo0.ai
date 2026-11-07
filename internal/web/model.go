package web

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/ui/report"
)

// modelView is one model's page: what its lab says, what lo0 found for
// this machine, where the memory goes and how fast a reply comes.
type modelView struct {
	Name, Meta, Summary, Card string
	Facts                     []fact
	GoodAt, WatchOut          []string
	Benchmarks                []benchmark
	Strengths, Concerns       []string
	// Needs is set for a model too large for this machine, which has none
	// of the fields below.
	Needs                  string
	Sessions, SessionTone  string
	SessionShare           float64
	Window                 string
	Memory                 []segment
	MemoryTotal            string
	Offload                []string
	Speed                  speedTable
	Race                   []raceRow
	Installed, InstalledAs string
	Options                []optionRow
	OptionsOffload         bool
	Try                    string
}

// benchmark is one published score from the model card.
type benchmark struct {
	Name, Score string
}

// segment is one part of a stacked memory bar.
type segment struct {
	Label, Size string
	Share       float64
	Tone        string
}

// optionRow is one way to run the model.
type optionRow struct {
	Quant, KV, Window, Sessions, Speed, Quality string
	InRAM                                       string
	Best                                        bool
}

func (s *Server) model(r *http.Request, snap *snapshot) (any, error) {
	m := snap.assessment.Match
	id := r.PathValue("id")
	for _, c := range m.Candidates {
		if c.Model.ID == id {
			return candidateView(c, m), nil
		}
	}
	for _, t := range m.TooLarge {
		if t.Model.ID == id {
			v := describe(t.Model)
			v.Needs = "Needs " + format.Bytes(uint64(t.NeedsBytes)) + " for its smallest usable build, and this machine has " + report.BudgetPhrase(m.Budget) + "."
			return v, nil
		}
	}
	return nil, errNotFound
}

// describe is what every model's page shows from the catalogue.
func describe(md catalog.Model) modelView {
	v := modelView{
		Name: md.Name, Meta: join([]string{md.Repo, md.License, released(md.Released)}),
		Summary: md.Profile.Summary, Card: md.Profile.Card,
		GoodAt: md.Profile.GoodAt, WatchOut: md.Profile.WatchOut,
	}
	for _, b := range md.Profile.Benchmarks {
		v.Benchmarks = append(v.Benchmarks, benchmark{Name: b.Name, Score: b.Score})
	}
	size := strconv.FormatFloat(float64(md.Params)/1e9, 'f', 0, 64) + "B parameters"
	if md.Params < 10e9 {
		size = strconv.FormatFloat(float64(md.Params)/1e9, 'f', 1, 64) + "B parameters"
	}
	if md.MoE() {
		size += ", " + strconv.FormatFloat(float64(md.StatedActive())/1e9, 'f', 1, 64) + "B active"
	}
	v.Facts = []fact{
		{"Size", []string{size}},
		{"Context", []string{format.Tokens(md.ContextLength) + " tokens"}},
		{"Tier", []string{string(md.Tier), md.TierNote}},
	}
	return v
}

func candidateView(c match.Candidate, m match.Result) modelView {
	v := describe(c.Model)
	v.Strengths, v.Concerns = c.Insights.Strengths, c.Insights.Concerns
	o := c.Best
	v.Sessions, v.SessionShare, v.SessionTone = format.Percent(o.Coverage), o.Coverage, coverageTone(o.Coverage)
	v.Window = format.Tokens(o.MaxContext)

	budget := float64(m.Budget.Bytes)
	if budget > 0 {
		v.Memory = []segment{
			{"weights", format.Bytes(uint64(o.WeightBytesOnGPU())), float64(o.WeightBytesOnGPU()) / budget, "accent"},
			{"KV cache", format.Bytes(uint64(o.KVBytes)), float64(o.KVBytes) / budget, "soft"},
			{"runtime", format.Bytes(match.RuntimeOverhead), float64(match.RuntimeOverhead) / budget, "faint"},
		}
	}
	v.MemoryTotal = format.Bytes(uint64(o.MemoryBytes)) + " of " + format.Bytes(m.Budget.Bytes)
	v.Offload = report.OffloadLines(c, m)
	v.Speed = speeds(c, m)
	v.Race = race(c.Model.Name, c.Pace, sourceNote(c), m.References)

	if in := c.Installed; in != nil {
		v.Installed = in.Tag + " in " + in.Runtime.Name()
		if !c.UsesInstalled() {
			v.InstalledAs = "lo0 recommends a different build from the one you have"
		}
	}
	for _, opt := range c.Options {
		kv := "f16"
		if opt.KV == catalog.KVQ8 {
			kv = "8-bit"
		}
		v.Options = append(v.Options, optionRow{
			Quant: opt.Quant.Name, KV: kv, Window: format.Tokens(opt.MaxContext),
			Sessions: format.Percent(opt.Coverage), Speed: strconv.Itoa(int(opt.TokensPerSecond)),
			Quality: format.Percent(opt.Quality), InRAM: report.OffloadShare(opt, c),
			Best: opt.Quant.Name == o.Quant.Name && opt.KV == o.KV && opt.Offload == o.Offload,
		})
		v.OptionsOffload = v.OptionsOffload || opt.Offloads()
	}
	v.Try = "lo0 try"
	if best, ok := m.Best(); !ok || best.Model.ID != c.Model.ID {
		v.Try += " --model " + c.Model.ID
	}
	return v
}

// speedTable is a model's writing and reading speeds at a few context
// sizes, and where the figures come from.
type speedTable struct {
	At, Write, Read []string
	Source          string
}

func speeds(c match.Candidate, m match.Result) speedTable {
	t := speedTable{Source: report.SourceLabel(c.Best.Curve)}
	if c.Best.Curve.Source == match.SourcePublished && m.SpeedBasis != "" {
		t.Source += ". " + m.SpeedBasis
	}
	for _, n := range report.SpeedPoints(c.Best, m.ReferenceContext) {
		at := format.Tokens(n)
		if n == m.ReferenceContext {
			at += " typical"
		}
		t.At = append(t.At, at)
		t.Write = append(t.Write, strconv.Itoa(int(m.SpeedAt(c, n))))
		t.Read = append(t.Read, format.Int(int(m.ReadSpeedAt(c, n))))
	}
	return t
}

// released turns a catalogue date such as "2026-04-15" into "April 2026".
func released(date string) string {
	t, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return strings.TrimSpace(date)
	}
	return t.Format("January 2006")
}
