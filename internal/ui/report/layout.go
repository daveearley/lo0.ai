package report

import (
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/ui/theme"
)

const (
	indent     = 2
	labelWidth = 14
	separator  = " · "
)

// doc accumulates report lines at a fixed width.
type doc struct {
	t     theme.Theme
	width int
	lines []string
}

func (d *doc) valueWidth() int {
	return max(d.width-indent-labelWidth, 10)
}

func (d *doc) blank() {
	d.lines = append(d.lines, "")
}

func (d *doc) heading(title string) {
	if len(d.lines) > 0 {
		d.blank()
	}
	d.lines = append(d.lines, d.t.Heading.Render(title))
}

// row writes a labelled line. value may already be styled.
func (d *doc) row(label, value string) {
	pad := strings.Repeat(" ", indent)
	l := d.t.Label.Render(ansi.Truncate(label, labelWidth-1, "")) + strings.Repeat(" ", max(labelWidth-lipgloss.Width(label), 1))
	d.lines = append(d.lines, pad+l+ansi.Truncate(value, d.valueWidth(), "…"))
}

// more writes an unlabelled continuation line under the value column.
func (d *doc) more(value string) {
	d.row("", value)
}

// lead writes a muted paragraph under a heading, wrapped to the full width
// rather than the value column, since it describes the whole section.
func (d *doc) lead(text string) {
	pad := strings.Repeat(" ", indent)
	for _, line := range strings.Split(Wrap(text, d.width-indent), "\n") {
		d.lines = append(d.lines, pad+d.t.Muted.Render(line))
	}
}

// prose writes text wrapped to the value column, labelled on its first line.
func (d *doc) prose(label, text string) {
	for i, line := range strings.Split(Wrap(text, d.valueWidth()), "\n") {
		if i == 0 {
			d.row(label, line)
		} else {
			d.more(line)
		}
	}
}

// bullet writes a wrapped list item with a hanging indent, labelled on its
// first line.
func (d *doc) bullet(label, text string) {
	marker := d.t.Muted.Render("-") + " "
	for i, line := range strings.Split(Wrap(text, d.valueWidth()-2), "\n") {
		if i == 0 {
			d.row(label, marker+line)
		} else {
			d.more("  " + line)
		}
	}
}

// note writes muted text across the full width, wrapped.
func (d *doc) note(text string) {
	for _, line := range strings.Split(Wrap(text, d.width), "\n") {
		d.lines = append(d.lines, d.t.Muted.Render(line))
	}
}

func (d *doc) String() string {
	return strings.Join(d.lines, "\n") + "\n"
}

// join renders parts separated by a muted middle dot.
func (d *doc) join(parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, d.t.Muted.Render(separator))
}

// fit joins as many items as fit in the value column, ending with "+N more"
// when some are left out.
func (d *doc) fit(items []string) string {
	width := d.valueWidth()
	var out []string
	for i, item := range items {
		candidate := d.join(append(out, item)...)
		rest := len(items) - i - 1
		suffix := ""
		if rest > 0 {
			suffix = d.t.Muted.Render(separator + "+" + format.Int(rest) + " more")
		}
		if lipgloss.Width(candidate+suffix) > width && len(out) > 0 {
			return d.join(out...) + d.t.Muted.Render(separator+"+"+format.Int(len(items)-i)+" more")
		}
		out = append(out, item)
	}
	return d.join(out...)
}

type align int

const (
	left align = iota
	right
)

// table lays out rows as aligned columns in the value column, with label on
// the first row. When the table would not fit, the first column shrinks and
// its cells are truncated with an ellipsis.
func (d *doc) table(label string, aligns []align, rows [][]string) {
	if len(rows) == 0 {
		return
	}
	const gap = 2
	widths := make([]int, len(aligns))
	for _, r := range rows {
		for i, cell := range r {
			widths[i] = max(widths[i], lipgloss.Width(cell))
		}
	}
	total := gap * (len(widths) - 1)
	for _, w := range widths {
		total += w
	}
	if over := total - d.valueWidth(); over > 0 {
		widths[0] = max(widths[0]-over, min(widths[0], 4))
	}
	for n, r := range rows {
		var b strings.Builder
		for i, cell := range r {
			if i > 0 {
				b.WriteString(strings.Repeat(" ", gap))
			}
			cell = ansi.Truncate(cell, widths[i], "…")
			pad := strings.Repeat(" ", widths[i]-lipgloss.Width(cell))
			switch {
			case aligns[i] == right:
				b.WriteString(pad + cell)
			case i < len(r)-1:
				b.WriteString(cell + pad)
			default:
				b.WriteString(cell)
			}
		}
		if n == 0 {
			d.row(label, b.String())
		} else {
			d.more(b.String())
		}
	}
}

// nonBreakingHyphen stands in for "-" while wrapping, because the wrapper
// always breaks after a hyphen and would split "trade-offs" or
// "Qwen3-Coder-30B" across lines.
const nonBreakingHyphen = "\u2011"

// nonBreakingSpace stands in for the spaces inside a lo0 command while
// wrapping, so a command the reader may copy is never split across lines.
const nonBreakingSpace = "\u00a0"

// lo0Command matches a lo0 command in prose: the subcommand and its flags with
// their values, up to the punctuation that ends the sentence.
var lo0Command = regexp.MustCompile(`lo0 (?:scan|match|try|migrate|benchmark|ui|version)(?: --?[a-z][a-z-]*(?: [^\s-][^\s,;:]*[^\s.,;:])?)*`)

// Wrap word-wraps text to width, breaking only at spaces outside lo0
// commands, and keeps ANSI styling intact.
func Wrap(text string, width int) string {
	text = lo0Command.ReplaceAllStringFunc(text, func(c string) string {
		return strings.ReplaceAll(c, " ", nonBreakingSpace)
	})
	text = strings.ReplaceAll(text, "-", nonBreakingHyphen)
	wrapped := ansi.Wrap(text, width, "")
	return strings.NewReplacer(nonBreakingHyphen, "-", nonBreakingSpace, " ").Replace(wrapped)
}
