package report

import (
	"math"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/match"
)

const maxCandidatesShown = 6

var levelLabels = map[match.Level]string{
	match.LevelReady:     "Ready",
	match.LevelTradeOffs: "Ready with trade-offs",
	match.LevelNotYet:    "Not yet",
	match.LevelNoHistory: "No history yet",
}

var areaLabels = map[harness.Area]string{
	harness.AreaInstructions: "Instructions",
	harness.AreaMCP:          "MCP servers",
	harness.AreaAppTools:     "App tools",
	harness.AreaHooks:        "Hooks",
	harness.AreaCommands:     "Commands",
	harness.AreaSubagents:    "Subagents",
	harness.AreaSkills:       "Skills",
	harness.AreaPlugins:      "Plugins",
	harness.AreaPermissions:  "Permissions",
	harness.AreaSettings:     "Settings",
}

// RenderMatch returns the match report: the verdict, the models that fit and
// what carries over to the target harness.
func RenderMatch(a app.Assessment, opts Options) string {
	res, m := a.Scan, a.Match
	width := opts.Width
	if width <= 0 {
		width = DefaultWidth
	}
	d := &doc{t: opts.Theme, width: min(width, MaxWidth)}

	title := d.t.Title.Render("lo0") + " " + d.t.Heading.Render("match")
	meta := d.t.Muted.Render(res.GeneratedAt.Local().Format("2 Jan 2006 15:04") + separator +
		format.Duration(time.Duration(res.DurationMS)*time.Millisecond))
	d.lines = append(d.lines, title+strings.Repeat(" ", max(d.width-lipgloss.Width(title)-lipgloss.Width(meta), 2))+meta)

	verdictSection(d, m.Verdict)
	modelsSection(d, m)
	projectsSection(d, m, opts.paths())
	costSection(d, a.Cost, res.Usage.First)
	carryoverSection(d, m)
	warningsSection(d, res.Warnings, opts.Hide)

	d.blank()
	where := " tokens, halfway through your median session."
	if res.Usage.Sessions == 0 {
		where = " tokens of context."
	}
	d.note("Speeds are estimated at " + format.Tokens(m.ReferenceContext) + where +
		" Token counts use Claude's tokenizer. Catalogue of " + m.CatalogDate + ".")
	return d.String()
}

func verdictSection(d *doc, v match.Verdict) {
	d.heading("Verdict")
	style := d.t.Warn
	switch v.Level {
	case match.LevelReady:
		style = d.t.Good
	case match.LevelNotYet:
		style = d.t.Bad
	default:
	}
	answer := style.Render(levelLabels[v.Level])
	if score := ScoreLabel(v); score != "" {
		answer += d.t.Muted.Render(separator + score)
	}
	d.row("Answer", answer)
	d.prose("", v.Headline)
	for i, reason := range v.Reasons {
		label := ""
		if i == 0 {
			d.blank()
			label = "Why"
		}
		d.bullet(label, reason)
	}
}

func modelsSection(d *doc, m match.Result) {
	d.heading("Models for this machine")
	bandwidth := strconv.Itoa(int(m.Bandwidth.GBps)) + " GB/s"
	if m.Bandwidth.Estimated {
		bandwidth += " (estimated)"
	}
	d.row("Memory", d.join(format.Bytes(m.Budget.Bytes)+" usable", bandwidth))

	if len(m.Candidates) == 0 {
		d.row("Fits", d.t.Bad.Render("none with a useful context window"))
	} else {
		// The pace column takes eleven columns; a narrow table shows only
		// the multiple, so model names keep their room.
		compact := d.valueWidth() < 64
		header := []string{"model", "window", "fits", "tok/s", "tier", "pace"}
		pace := PaceLabel
		if compact {
			header[5], pace = "slower", PaceMultiple
		}
		for i, h := range header {
			header[i] = d.t.Muted.Render(h)
		}
		shown := m.Candidates[:min(len(m.Candidates), maxCandidatesShown)]
		rows := make([][]string, 0, 1+len(shown))
		rows = append(rows, header)
		for _, c := range shown {
			name := c.Model.Name
			// With no history there is no share to state.
			fits := d.t.Muted.Render("-")
			if m.Verdict.Level != match.LevelNoHistory {
				fits = coverage(d, c.Best.Coverage)
			}
			if c.Installed != nil {
				name += d.t.Accent.Render(" ✓")
			}
			rows = append(rows, []string{
				name,
				format.Tokens(c.Best.MaxContext),
				fits,
				strconv.Itoa(int(c.Best.TokensPerSecond)),
				string(c.Model.Tier),
				pace(c.Pace, d.t),
			})
		}
		paceAlign := left
		if compact {
			paceAlign = right
		}
		d.table("Fits", []align{left, right, right, right, right, paceAlign}, rows)
		if extra := len(m.Candidates) - maxCandidatesShown; extra > 0 {
			d.more(d.t.Muted.Render("and " + format.Plural(extra, "more model", "more models")))
		}
		if hasInstalled(m.Candidates) {
			d.more(d.t.Muted.Render("✓ already installed"))
		}
		if len(m.References) > 0 {
			r := m.References[0]
			against := "pace against "
			if compact {
				against = "slower than "
			}
			for l := range strings.SplitSeq(Wrap(against+r.Name+" in your "+r.Harness+" sessions, "+
				strconv.Itoa(int(math.Round(r.TokensPerSecond)))+" tok/s", d.valueWidth()), "\n") {
				d.more(d.t.Muted.Render(l))
			}
		}
		var measured []string
		for _, c := range shown {
			if c.Best.Curve.Source == match.SourceMeasured {
				measured = append(measured, c.Model.Name)
			}
		}
		note := "tok/s " + SourceLabel(match.Curve{Source: m.SpeedSource})
		if len(measured) > 0 {
			note += ", except " + strings.Join(measured, " and ") + ", measured here"
		}
		for l := range strings.SplitSeq(Wrap(note, d.valueWidth()), "\n") {
			d.more(d.t.Muted.Render(l))
		}

	}

	if bg := m.Background; bg != nil {
		d.row("Background", d.join(bg.Model.Name+" at "+bg.Option.Quant.Name, format.Tokens(bg.Option.MaxContext)+" window",
			strconv.Itoa(int(bg.Option.TokensPerSecond))+" tok/s"))
		if best, ok := m.Best(); ok {
			d.prose("", d.t.Muted.Render("for titles, summaries and quick subagents, beside "+best.Model.Name))
		}
	}
	if n := len(m.TooLarge); n > 0 {
		smallest := m.TooLarge[0]
		d.prose("Too large", format.Plural(n, "model needs", "models need")+" more memory, starting with "+
			smallest.Model.Name+" at "+unbreakable(format.Bytes(uint64(smallest.NeedsBytes)))+".")
	}
	if len(m.Uncatalogued) > 0 {
		names := make([]string, 0, len(m.Uncatalogued))
		for _, u := range m.Uncatalogued {
			names = append(names, displayName(u.Name))
		}
		verb, pronoun := "is", "its"
		if len(names) > 1 {
			verb, pronoun = "are", "their"
		}
		d.prose("Unknown", strings.Join(names, ", ")+d.t.Muted.Render(" "+verb+" installed but not in the catalogue, so lo0 cannot work out "+pronoun+" fit."))
	}
}

func coverage(d *doc, share float64) string {
	return fitStyle(d, share)(format.Percent(share))
}

func hasInstalled(cs []match.Candidate) bool {
	for _, c := range cs[:min(len(cs), maxCandidatesShown)] {
		if c.Installed != nil {
			return true
		}
	}
	return false
}

func carryoverSection(d *doc, m match.Result) {
	if m.Target == "" {
		return
	}
	d.heading("Carries over to " + m.Target)
	if harness.Summarize(m.Features, "").Total() == 0 {
		d.lead("lo0 found no Claude Code setup to carry over: no instruction files, MCP servers, hooks, skills or settings. " +
			m.Target + " starts as it comes.")
		return
	}
	for _, area := range harness.Areas {
		c := harness.Summarize(m.Features, area)
		if c.Total() == 0 {
			continue
		}
		parts := carryoverParts(d, c)
		if area == harness.AreaAppTools {
			calls := 0
			for _, f := range m.Features {
				if f.Area == area {
					calls += f.Calls
				}
			}
			parts = append(parts, d.t.Muted.Render(format.Plural(calls, "call", "calls")+" in your history"))
		}
		d.row(areaLabels[area], d.join(parts...))
	}
	for _, t := range m.Targets {
		if t.ID == m.TargetID {
			continue
		}
		c := harness.Summarize(t.Features, "")
		if c.Total() == 0 {
			continue
		}
		d.row("Instead", t.Name+": "+d.join(carryoverParts(d, c)...))
	}
}

func carryoverParts(d *doc, c harness.Carryover) []string {
	var parts []string
	if c.Native > 0 {
		parts = append(parts, d.t.Good.Render(format.Int(c.Native)+" as is"))
	}
	if c.Translate > 0 {
		parts = append(parts, format.Int(c.Translate)+" converted")
	}
	if c.Partial > 0 {
		parts = append(parts, d.t.Warn.Render(format.Int(c.Partial)+" partly"))
	}
	if c.Lost > 0 {
		parts = append(parts, d.t.Bad.Render(format.Int(c.Lost)+" lost"))
	}
	return parts
}

// unbreakable joins a number and its unit so that wrapping keeps them on one
// line.
func unbreakable(s string) string {
	return strings.ReplaceAll(s, " ", "\u00a0")
}

// LevelLabel names a verdict level as every front end shows it, such as
// "Ready with trade-offs".
func LevelLabel(l match.Level) string {
	return levelLabels[l]
}

// ScoreLabel states the verdict's readiness score, or is empty when there
// is no history to score.
func ScoreLabel(v match.Verdict) string {
	if v.Level == match.LevelNoHistory {
		return ""
	}
	return "readiness " + format.Int(v.Score) + " of 100"
}
