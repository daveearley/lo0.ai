package report

import (
	"strconv"
	"strings"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/ui/draw"
)

// contextBarCells is the width of the bar showing how much of the window a
// session used.
const contextBarCells = 20

// RenderSession reports what a session started by lo0 try did: how long it
// ran, how much of the window it used, how fast the model really wrote and
// how often its tool calls failed, each beside what the user is used to.
func RenderSession(r app.SessionReport, opts Options) string {
	d := detailDoc(opts)
	if !opts.Titled {
		d.heading("Session")
	}
	ended := "exited normally"
	if r.ExitCode != 0 {
		ended = "exited with status " + strconv.Itoa(r.ExitCode)
	}
	length := format.Duration(r.Ended.Sub(r.Started).Round(1e9))
	if !r.Found {
		d.row("Ran", d.join(length, ended))
		if r.ExitCode != 0 {
			d.prose("", r.Harness+" stopped with an error before sending a request to "+r.Model+"; what it printed is in the terminal it ran in.")
			return d.String()
		}
		d.prose("", "No requests to "+r.Model+" were found in the logs lo0 reads, so only how long the session ran is known.")
		return d.String()
	}
	d.row("Ran", d.join(length, format.Plural(r.Requests, "request", "requests"), ended))

	if r.Window > 0 {
		filled, track := draw.Bar(float64(r.PeakContext)/float64(r.Window), contextBarCells)
		d.row("Context", format.Tokens(r.PeakContext)+" of "+format.Tokens(r.Window)+"  "+
			d.t.Accent.Render(filled)+d.t.Muted.Render(track))
	}

	switch {
	case r.Speed > 0:
		d.row("Speed", strconv.Itoa(int(r.Speed))+" tok/s measured over "+format.Plural(r.Replies, "reply", "replies"))
		if against := measuredPace(d, r); against != "" {
			for l := range strings.SplitSeq(Wrap(against, d.valueWidth()), "\n") {
				d.more(l)
			}
		}
		// Replies are timed from the request, as the reference's are, so
		// the wait to read the context is in both; the estimate leaves it
		// out.
		if r.Estimate > 0 {
			note := "The time includes reading the context before writing; lo0's estimate of " +
				strconv.Itoa(int(r.Estimate)) + " tok/s covers writing alone."
			for l := range strings.SplitSeq(Wrap(note, d.valueWidth()), "\n") {
				d.more(d.t.Muted.Render(l))
			}
		}
		// The same race Home draws from the estimate, now from what this
		// session measured.
		for i, l := range race(r.Model, r.Pace, "this session", r.References, d.t, d.valueWidth()) {
			if i == 0 {
				d.row("A reply", l)
			} else {
				d.more(l)
			}
		}
	default:
		d.row("Speed", d.t.Muted.Render("no reply was long enough to time"))
	}

	if r.ToolResults > 0 {
		share := float64(r.ToolErrors) / float64(r.ToolResults)
		line := format.Plural(r.ToolResults, "call", "calls") + " · " + strconv.Itoa(r.ToolErrors) + " failed"
		if r.UsualToolErrors > 0 {
			style := d.t.Value
			if share > 2*r.UsualToolErrors {
				style = d.t.Warn
			}
			line += d.t.Muted.Render(", ") + style.Render(format.Percent(share)) +
				d.t.Muted.Render(" against "+format.Percent(r.UsualToolErrors)+" in your history")
		}
		d.row("Tool calls", line)
	}
	if r.Compactions > 0 {
		d.row("Compactions", strconv.Itoa(r.Compactions))
	}
	return d.String()
}

// measuredPace says how the measured speed compares with the reference.
func measuredPace(d *doc, r app.SessionReport) string {
	if len(r.References) == 0 || r.Pace.Slower == 0 {
		return ""
	}
	ref := r.References[0]
	label := PaceLabel(r.Pace, d.t)
	if r.Pace.Band == match.BandVerySlow {
		label += d.t.Bad.Render(", " + format.Multiple(r.Pace.Slower) + " slower")
	}
	if r.Pace.Slower < match.AboutAsFast {
		return label + d.t.Muted.Render(" as "+ref.Name+" in your "+ref.Harness+" sessions")
	}
	return label + d.t.Muted.Render(" than "+ref.Name+" in your "+ref.Harness+" sessions")
}
