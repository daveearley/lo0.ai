package report

import (
	"math"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/ui/draw"
	"lo0.ai/lo0/internal/ui/theme"
)

// PaceLabel names a candidate's speed against what the user is used to in
// a few words, coloured by its band: "2.9× slower" in amber, "very slow" in
// red. The words carry the meaning without colour.
func PaceLabel(p match.Pace, t theme.Theme) string {
	switch {
	case p.Band == match.BandVerySlow:
		return t.Bad.Render("very slow")
	case p.Band == match.BandSlower && p.Slower > 0:
		return t.Warn.Render(format.Multiple(p.Slower) + " slower")
	case p.Band == match.BandSlower:
		return t.Warn.Render("slow")
	case p.Slower >= match.AboutAsFast:
		return t.Value.Render(format.Multiple(p.Slower) + " slower")
	case p.Slower > 0:
		return t.Value.Render("about as fast")
	default:
		return ""
	}
}

// PaceMultiple is PaceLabel for a narrow table: the multiple alone, such as
// "2.9×", coloured by band, or the band's word when there is no reference.
func PaceMultiple(p match.Pace, t theme.Theme) string {
	if p.Slower == 0 {
		if p.Band == match.BandComparable {
			return ""
		}
		return paceStyle(t, p.Band).Render("slow")
	}
	return paceStyle(t, p.Band).Render(format.Multiple(p.Slower))
}

// paceStyle is the colour of a band.
func paceStyle(t theme.Theme, b match.Band) lipgloss.Style {
	switch b {
	case match.BandVerySlow:
		return t.Bad
	case match.BandSlower:
		return t.Warn
	default:
		return t.Value
	}
}

// SpeedComparison draws a candidate's estimated speed against the speeds
// the user gets from hosted models, on one scale, with the time each takes
// over a typical reply, then says where the figures come from. It returns
// nothing when there is no reference to compare with.
func SpeedComparison(c match.Candidate, refs []match.Reference, reference int, t theme.Theme, width int) []string {
	if len(refs) == 0 {
		return nil
	}
	type line struct {
		name string
		// speed is shown as printed: references rounded, the estimate cut
		// to whole tokens as everywhere else lo0 shows it.
		speed   int
		seconds float64
		style   lipgloss.Style
		note    string
	}
	lines := make([]line, 0, len(refs)+1)
	top := c.Best.TokensPerSecond
	for _, r := range refs {
		lines = append(lines, line{r.Name + " in " + r.Harness, int(math.Round(r.TokensPerSecond)), r.ReplySeconds, t.Label, ""})
		top = max(top, r.TokensPerSecond)
	}
	note := "estimated"
	if c.Best.Curve.Source == match.SourceMeasured {
		note = "measured"
	}
	lines = append(lines, line{c.Model.Name, int(c.Best.TokensPerSecond), c.Pace.ReplySeconds, paceStyle(t, c.Pace.Band), note})

	nameW := 0
	for _, l := range lines {
		nameW = max(nameW, lipgloss.Width(l.name))
	}
	nameW = min(nameW, 24)
	// After the bar: the speed, the seconds and the estimate's note.
	const tail = 2 + 9 + 2 + 6 + 11
	cells := min(max(width-nameW-2-tail, 8), 40)
	out := make([]string, 0, len(lines)+2)
	for _, l := range lines {
		filled, _ := draw.Bar(float64(l.speed)/top, cells)
		row := t.Value.Render(pad(ansi.Truncate(l.name, nameW, "…"), nameW, false)) + "  " +
			l.style.Render(pad(filled, cells, false)) + "  " +
			t.Value.Render(pad(strconv.Itoa(l.speed), 3, true)) + t.Muted.Render(" tok/s") + "  " +
			t.Value.Render(pad(format.Seconds(l.seconds), 6, true))
		if l.note != "" {
			row += t.Muted.Render("  " + l.note)
		}
		out = append(out, row)
	}
	main := refs[0]
	names := make([]string, 0, len(refs))
	for _, r := range refs {
		names = append(names, format.Int(r.Replies)+" "+r.Name)
	}
	typical := "writing " + format.Int(main.ReplyTokens) + " tokens"
	if main.NewTokens > 0 {
		typical = "reading " + format.Int(main.NewTokens) + " new tokens and " + typical
	}
	source := "Seconds are for a typical reply, " + typical + ", the medians in your " + main.Harness +
		" sessions at " + format.Tokens(reference) + " of context. Hosted speeds are measured from " + strings.Join(names, " and ") +
		" replies; " + c.Model.Name + "'s are " + SourceLabel(c.Best.Curve) + "."
	for l := range strings.SplitSeq(Wrap(source, width), "\n") {
		out = append(out, t.Muted.Render(l))
	}
	return out
}

// pad widens plain s to w columns, on the left when right-aligning.
func pad(s string, w int, right bool) string {
	gap := strings.Repeat(" ", max(w-lipgloss.Width(s), 0))
	if right {
		return gap + s
	}
	return s + gap
}

// ReplyRace draws how long a typical reply takes from each hosted model the
// user works with and from c, longest bar slowest, since the wait is what
// people feel. It returns nothing without a reference.
func ReplyRace(c match.Candidate, refs []match.Reference, t theme.Theme, width int) []string {
	note := "estimated"
	if c.Best.Curve.Source == match.SourceMeasured {
		note = "measured here"
	}
	return race(c.Model.Name, c.Pace, note, refs, t, width)
}

// race draws the reply race between the references and a local model whose
// pace against them is p, with note after the local model's time.
func race(name string, p match.Pace, note string, refs []match.Reference, t theme.Theme, width int) []string {
	if len(refs) == 0 || p.ReplySeconds <= 0 {
		return nil
	}
	type row struct {
		name    string
		seconds float64
		style   lipgloss.Style
		note    string
	}
	rows := make([]row, 0, len(refs)+1)
	longest := p.ReplySeconds
	for _, r := range refs {
		rows = append(rows, row{r.Name + " · " + r.Harness, r.ReplySeconds, t.Label, ""})
		longest = max(longest, r.ReplySeconds)
	}
	rows = append(rows, row{name, p.ReplySeconds, paceStyle(t, p.Band), note})

	nameW, noteW := 0, 0
	for _, r := range rows {
		nameW = max(nameW, lipgloss.Width(r.name))
		noteW = max(noteW, lipgloss.Width(r.note))
	}
	nameW = min(nameW, 24)
	// After the bar: the seconds and the note.
	tail := 2 + 6 + 2 + noteW
	cells := min(max(width-nameW-2-tail, 4), 40)
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		filled, _ := draw.Bar(r.seconds/longest, cells)
		line := t.Value.Render(pad(ansi.Truncate(r.name, nameW, "…"), nameW, false)) + "  " +
			r.style.Render(pad(filled, cells, false)) + "  " +
			t.Value.Render(pad(format.Seconds(r.seconds), 6, true))
		if r.note != "" {
			line += t.Muted.Render("  " + r.note)
		}
		out = append(out, line)
	}
	return out
}
