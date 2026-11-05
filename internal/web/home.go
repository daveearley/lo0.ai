package web

import (
	"math"
	"net/http"
	"strings"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/runtimes"
)

// daysPerMonth turns a history's span into months.
const daysPerMonth = 30.44

// homeView is the Home page: how much of the user's work fits, how a reply
// compares, where their commands go and what to do next.
type homeView struct {
	// Share is the part of the work the best model fits, such as "65%",
	// and Tone its colour. Both are empty when no model fits.
	Share, Tone string
	Fit, Rest   string
	Curve       curve
	HasCurve    bool
	// Verdict stands in for the share when nothing fits.
	Verdict  string
	Race     []raceRow
	Routes   []routeRow
	Kept     string
	LastTry  string
	Headline string
	Steps    []stepRow
	Facts    []fact
}

// raceRow is one model in "a typical reply": how long it takes.
type raceRow struct {
	Name, Seconds, Note, Tone string
	Share                     float64
}

// routeRow is one command the user starts a harness with and the model it
// reaches.
type routeRow struct {
	Command, Model string
	Local          bool
}

// stepRow is one suggested next step, with the command that does it or a
// page that shows it.
type stepRow struct {
	Label, Note, Command, Href, Link string
}

// fact is one labelled column of short lines.
type fact struct {
	Label string
	Lines []string
}

func (s *Server) home(_ *http.Request, snap *snapshot) (any, error) {
	a := snap.assessment
	v := homeView{Headline: snap.next.Headline}
	if snap.next.Status != "" {
		v.Headline = snap.next.Status
	}
	if best, ok := a.Match.Best(); ok {
		peaks := a.Scan.Usage.Context.Peaks
		share := best.Best.Coverage
		v.Share, v.Tone = format.Percent(share), coverageTone(share)
		if c, ok := contextCurve(peaks, best.Best.MaxContext); ok {
			v.Curve, v.HasCurve = c, true
			fit := int(math.Round(c.Split * float64(len(peaks))))
			v.Fit = format.Plural(fit, "session fits", "sessions fit") + " in " + best.Model.Name + "’s " + format.Tokens(best.Best.MaxContext) + " window"
			if rest := len(peaks) - fit; rest > 0 {
				v.Rest = format.Int(rest) + " would compact"
			}
		}
		v.Race = race(best.Model.Name, best.Pace, sourceNote(best), a.Match.References)
	} else {
		v.Verdict = a.Match.Verdict.Headline
	}
	for _, h := range snap.harnesses {
		for _, u := range h.Installation.Uses {
			v.Routes = append(v.Routes, routeRow{Command: u.Command, Model: u.Model, Local: u.Local})
		}
	}
	if len(v.Routes) > 0 && snap.next.Kept == "" {
		v.Kept = "no local model kept yet"
	}
	if !snap.next.LastTry.IsZero() {
		v.LastTry = "last tried " + snap.next.LastTryModel + " on " + snap.next.LastTry.Format("2 Jan")
	}
	for _, st := range snap.next.Steps {
		v.Steps = append(v.Steps, step(st, a))
	}
	v.Facts = []fact{thisMachine(snap), yourClaude(a)}
	return v, nil
}

// coverageTone colours a share of sessions as the terminal does.
func coverageTone(share float64) string {
	switch match.FitOf(share) {
	case match.FitGood:
		return "good"
	case match.FitPartial:
		return "warn"
	default:
		return "bad"
	}
}

// paceTone colours a speed band.
func paceTone(b match.Band) string {
	switch b {
	case match.BandVerySlow:
		return "bad"
	case match.BandSlower:
		return "warn"
	default:
		return "good"
	}
}

func sourceNote(c match.Candidate) string {
	if c.Best.Curve.Source == match.SourceMeasured {
		return "measured here"
	}
	return "estimated"
}

// race is how long a typical reply takes from each hosted model the user
// works with and from a local model whose pace against them is p, scaled
// to the slowest.
func race(name string, p match.Pace, note string, refs []match.Reference) []raceRow {
	if len(refs) == 0 || p.ReplySeconds <= 0 {
		return nil
	}
	longest := p.ReplySeconds
	for _, r := range refs {
		longest = max(longest, r.ReplySeconds)
	}
	out := make([]raceRow, 0, len(refs)+1)
	for _, r := range refs {
		out = append(out, raceRow{Name: r.Name + " · " + r.Harness, Seconds: format.Seconds(r.ReplySeconds), Share: r.ReplySeconds / longest, Tone: "ref"})
	}
	return append(out, raceRow{Name: name, Seconds: format.Seconds(p.ReplySeconds), Share: p.ReplySeconds / longest, Tone: paceTone(p.Band), Note: note})
}

// step turns a next step into what the page offers: the command that
// takes it, or the page that shows it. Commands leave out what lo0 would
// choose anyway, so the best step reads as plainly as "lo0 try".
func step(st app.NextStep, a app.Assessment) stepRow {
	row := stepRow{Label: st.Label, Note: st.Note}
	switch st.Kind {
	case app.StepTry:
		row.Command = "lo0 try" + choice(st, a.Match, "--harness")
	case app.StepKeep:
		row.Command = "lo0 migrate" + choice(st, a.Match, "--to")
	case app.StepCompare:
		row.Href, row.Link = "/models/"+st.Model, "Open"
	case app.StepBrowse:
		row.Href, row.Link = "/models?show=all", "Open"
	default:
		// Downloads, installs and removals are planned and confirmed in
		// the interactive interface.
		row.Command = "lo0 ui"
	}
	return row
}

// choice is the flags that pick a step's harness, runtime and model, each
// left out when it is what lo0 picks by default: the recommended harness,
// the best model and the runtime that already has it.
func choice(st app.NextStep, m match.Result, harnessFlag string) string {
	var b strings.Builder
	if st.Harness != "" && st.Harness != m.TargetID {
		b.WriteString(" " + harnessFlag + " " + string(st.Harness))
	}
	model := st.Model
	if model == "" {
		if best, ok := m.Best(); ok {
			model = best.Model.ID
		}
	}
	if st.Runtime != "" && st.Runtime != installedIn(m, model) {
		b.WriteString(" --runtime " + string(st.Runtime))
	}
	if best, ok := m.Best(); st.Model != "" && (!ok || best.Model.ID != st.Model) {
		b.WriteString(" --model " + st.Model)
	}
	return b.String()
}

// installedIn is the runtime that has the model with id, if any.
func installedIn(m match.Result, id string) runtimes.ID {
	for _, c := range m.Candidates {
		if c.Model.ID == id && c.Installed != nil {
			return c.Installed.Runtime
		}
	}
	return ""
}

func thisMachine(snap *snapshot) fact {
	a := snap.assessment
	models := 0
	for _, rt := range a.Scan.Runtimes {
		for _, m := range rt.Models {
			if m.Parent == "" {
				models++
			}
		}
	}
	line := format.Plural(models, "model", "models")
	if b := snap.inventory.DiskBytes; b > 0 {
		line += " · " + format.Bytes(b)
	}
	lines := []string{line}
	running := "no runtime running"
	for _, rt := range a.Scan.Runtimes {
		if rt.Running {
			running = rt.Name + " running"
			break
		}
	}
	lines = append(lines, running)
	if free := a.Scan.Machine.Disk.FreeBytes; free > 0 {
		lines = append(lines, format.Bytes(free)+" free for downloads")
	}
	return fact{"This machine", lines}
}

func yourClaude(a app.Assessment) fact {
	p := a.Scan.Usage
	if p.Sessions == 0 {
		return fact{"Your Claude", []string{"no sessions found"}}
	}
	sessions := format.Plural(p.Sessions, "session", "sessions")
	if months := p.Last.Sub(p.First).Hours() / 24 / daysPerMonth; months >= 1 {
		sessions += " in " + format.Plural(int(math.Round(months)), "month", "months")
	}
	lines := []string{sessions}
	if usd := a.Cost.TotalUSD; usd > 0 {
		lines = append(lines, format.USD(usd)+" at API prices")
	}
	if usd := a.Cost.Last30DaysUSD; usd > 0 {
		lines = append(lines, format.USD(usd)+" in the last 30 days")
	}
	return fact{"Your Claude", lines}
}
