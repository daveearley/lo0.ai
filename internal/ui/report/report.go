// Package report renders a scan result as a terminal report. Rendering is a
// pure function of the result, so the same text is shown live, written to a
// file and checked by golden tests.
package report

import (
	"path/filepath"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"lo0.ai/lo0/internal/claudecode"
	"lo0.ai/lo0/internal/codex"
	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/scan"
	"lo0.ai/lo0/internal/ui/theme"
)

// DefaultWidth is used when the terminal width is unknown.
const DefaultWidth = 80

// MaxWidth caps line length on wide terminals, where long lines get hard to
// scan.
const MaxWidth = 100

// Options control rendering.
type Options struct {
	Width int
	Theme theme.Theme
	// Home is the user's home directory. Paths under it are shown with "~".
	Home string
	// Hide, when set, rewrites every path shown, for reports that will be
	// shared. It replaces the "~" shortening, so it must handle home too.
	Hide func(string) string
	// Titled is set when the text appears under a title of its own, as in
	// the interactive interface, so a renderer leaves out its heading.
	Titled bool
}

// paths returns the function every path shown passes through.
func (o Options) paths() func(string) string {
	if o.Hide != nil {
		return o.Hide
	}
	return pathShortener(o.Home)
}

// Render returns the report for res.
func Render(res scan.Result, opts Options) string {
	width := opts.Width
	if width <= 0 {
		width = DefaultWidth
	}
	d := &doc{t: opts.Theme, width: min(width, MaxWidth)}
	paths := opts.paths()

	header(d, res)
	machineSection(d, res.Machine)
	runtimesSection(d, res.Runtimes)
	setupSection(d, res.Setup, res.Claude, res.Usage.Of(claudecode.Harness).Format, paths)
	codexSection(d, res.Codex, pooled(res.Usage, codex.Harness).Format, paths)
	historySection(d, res.Usage, res.Layout.ProjectsDir(), paths)
	warningsSection(d, res.Warnings, opts.Hide)
	return d.String()
}

func header(d *doc, res scan.Result) {
	title := d.t.Title.Render("lo0") + " " + d.t.Heading.Render("scan")
	meta := d.t.Muted.Render(res.GeneratedAt.Local().Format("2 Jan 2006 15:04") + separator +
		format.Duration(time.Duration(res.DurationMS)*time.Millisecond))
	gap := max(d.width-lipgloss.Width(title)-lipgloss.Width(meta), 2)
	d.lines = append(d.lines, title+strings.Repeat(" ", gap)+meta)
}

func warningsSection(d *doc, warnings []scan.Warning, hide func(string) string) {
	if len(warnings) == 0 {
		return
	}
	d.heading(format.Plural(len(warnings), "warning", "warnings"))
	// A warning is wrapped, never cut: it is the one thing the reader must
	// be able to read whole. Paths are hidden before wrapping, because a
	// path broken across lines no longer matches what Hide looks for.
	for _, w := range warnings {
		msg := w.Message
		if hide != nil {
			msg = hide(msg)
		}
		for i, line := range strings.Split(Wrap(msg, d.valueWidth()), "\n") {
			label := ""
			if i == 0 {
				label = string(w.Stage)
			}
			d.row(label, d.t.Warn.Render(line))
		}
	}
}

// pathShortener replaces the home directory prefix with "~".
func pathShortener(home string) func(string) string {
	return func(p string) string {
		if home == "" {
			return p
		}
		rel, err := filepath.Rel(home, p)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return p
		}
		if rel == "." {
			return "~"
		}
		return "~" + string(filepath.Separator) + rel
	}
}
