package report

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/migrate"
)

// RenderPlan returns a migration plan as the user reviews it before
// anything changes: what it moves to, every step with its specifics, what is
// left to do by hand and how to start afterwards.
func RenderPlan(p migrate.Plan, opts Options) string {
	d := detailDoc(opts)
	if p.Undo {
		d.lines = append(d.lines, d.t.Title.Render("lo0")+" "+d.t.Heading.Render("migrate --undo"))
		d.blank()
		d.note("Reverses what lo0 migrate changed, newest first. Files you have edited since are left as they are.")
	} else {
		if !opts.Titled {
			d.lines = append(d.lines, d.t.Title.Render("lo0")+" "+d.t.Heading.Render(command(p)))
		}
		planSummary(d, p)
	}

	d.heading("Steps")
	n := 0
	for _, s := range p.Steps {
		mark := d.t.Good.Render("✓")
		if !s.Done {
			n++
			mark = d.t.Accent.Render(strconv.Itoa(n))
		}
		stepLines(d, s, mark, opts)
	}
	session(d, p, opts)
	byHand(d, p, opts)
	then(d, p, opts)
	return d.String()
}

// RenderOutcome reports what Apply did to each step, what the user still
// has to do, and how to start once everything is done.
func RenderOutcome(p migrate.Plan, out migrate.Outcome, opts Options) string {
	d := detailDoc(opts)
	d.heading("Result")
	for i, s := range p.Steps {
		state := migrate.StateSkipped
		if i < len(out.States) && out.States[i] != "" {
			state = out.States[i]
		}
		d.lines = append(d.lines, strings.Repeat(" ", indent)+StepMark(state, opts)+" "+stepTitle(d, s, state))
	}
	if waiting := out.Waiting(); len(waiting) > 0 {
		d.heading("Still for you to do")
		for _, i := range waiting {
			stepLines(d, p.Steps[i], d.t.Warn.Render("→"), opts)
		}
		d.blank()
		d.note("Then run lo0 " + command(p) + " again to finish.")
		return d.String()
	}
	if out.Complete() {
		measured(d, p)
		then(d, p, opts)
	}
	return d.String()
}

// measured lists what a benchmark measured: at each context, how fast the
// model read the prompt and then wrote its reply.
func measured(d *doc, p migrate.Plan) {
	if p.Measured == nil || len(p.Measured.Points) == 0 {
		return
	}
	d.heading("Measured")
	rows := [][]string{{d.t.Label.Render("context"), d.t.Label.Render("reading"), d.t.Label.Render("writing")}}
	for _, pt := range p.Measured.Points {
		rows = append(rows, []string{
			format.Tokens(pt.Context),
			format.Int(int(pt.ReadTokensPerSecond)) + d.t.Muted.Render(" tok/s"),
			d.t.Accent.Render(format.Int(int(pt.WriteTokensPerSecond))) + d.t.Muted.Render(" tok/s"),
		})
	}
	d.table("", []align{left, right, right}, rows)
	d.blank()
	d.note("Every speed lo0 shows for " + p.Model + " on this machine is measured from now on.")
}

// StepMark is the one-character marker for a step's state.
func StepMark(s migrate.State, opts Options) string {
	t := opts.Theme
	switch s {
	case migrate.StateDone, migrate.StateAlready:
		return t.Good.Render("✓")
	case migrate.StateWaiting:
		return t.Warn.Render("→")
	case migrate.StateFailed:
		return t.Bad.Render("✕")
	case migrate.StateRunning, migrate.StateProgress:
		return t.Accent.Render("·")
	default:
		return t.Muted.Render("–")
	}
}

// Progress describes a running step's progress in a few words, such as
// "41% of 18.6 GiB" for a download.
func Progress(s migrate.Step, done, total int64) string {
	if total <= 0 {
		return ""
	}
	pct := strconv.Itoa(int(100*done/total)) + "%"
	switch s.Kind {
	case migrate.KindDownload:
		return pct + " of " + format.Bytes(uint64(total))
	case migrate.KindMeasure:
		return format.Int(int(done)) + " of " + format.Int(int(total)) + " tokens written"
	default:
	}
	return pct
}

func stepTitle(d *doc, s migrate.Step, state migrate.State) string {
	title := s.Title
	if s.Kind == migrate.KindManual && !s.Done {
		title = "You: " + title
	}
	if state == migrate.StateAlready {
		return d.t.Muted.Render(title + separator + "already done")
	}
	if state == migrate.StateSkipped {
		return d.t.Muted.Render(title + separator + "waits for the steps above")
	}
	return title
}

// stepLines writes a step with its details under it. Details are commands
// and paths the user may copy, so they are never truncated.
func stepLines(d *doc, s migrate.Step, mark string, opts Options) {
	title := s.Title
	if s.Kind == migrate.KindManual && !s.Done {
		title = d.t.Warn.Render("You") + d.t.Muted.Render(separator) + title
	}
	if s.Done {
		title = d.t.Muted.Render(title)
	}
	size := ""
	if s.Bytes > 0 && !s.Done {
		size = d.t.Muted.Render(format.Bytes(uint64(s.Bytes)))
	}
	pad := strings.Repeat(" ", indent)
	line := pad + mark + "  " + title
	if size != "" {
		line += strings.Repeat(" ", max(d.width-lipgloss.Width(line)-lipgloss.Width(size), 2)) + size
	}
	d.lines = append(d.lines, line)
	if s.Done {
		return
	}
	for _, detail := range s.Detail {
		d.lines = append(d.lines, pad+"   "+d.t.Muted.Render(opts.paths()(detail)))
	}
}

// command is the lo0 command a plan comes from.
func command(p migrate.Plan) string {
	switch {
	case p.Command != "":
		return p.Command
	case p.Session != nil:
		return "try"
	default:
		return "migrate"
	}
}

// session writes the command a try starts once its steps have run and
// what the session is given.
func session(d *doc, p migrate.Plan, opts Options) {
	if p.Session == nil {
		return
	}
	d.heading("Session")
	pad := strings.Repeat(" ", indent)
	d.lines = append(d.lines, pad+"Start "+d.t.Accent.Render(p.Session.Command())+d.t.Muted.Render(" once the steps are done"))
	for _, detail := range p.Session.Detail {
		d.lines = append(d.lines, pad+"  "+d.t.Muted.Render(opts.paths()(detail)))
	}
}

func planSummary(d *doc, p migrate.Plan) {
	d.heading("Plan")
	if p.HarnessName != "" {
		d.row("Harness", p.HarnessName)
	}
	window := ""
	if p.Context > 0 {
		window = format.Tokens(p.Context) + " window"
	}
	if p.Model != "" {
		d.row("Model", d.join(p.Model, p.Quant, window))
	}
	if bg := p.Served.Background; bg != nil {
		d.row("Background", d.join(bg.DisplayName, bg.Quant, format.Tokens(bg.Context)+" window"))
	}
	if p.RuntimeName != "" {
		d.row("Runtime", d.join(p.RuntimeName, p.Served.BaseURL))
	}
	if p.Served.Model != "" {
		d.row("Served as", p.Served.Model)
	}
	if bg := p.Served.Background; bg != nil {
		d.more(bg.Model)
	}
	if p.Served.BackgroundNote != "" {
		d.prose("Background", d.t.Muted.Render(p.Served.BackgroundNote))
	}
}

func byHand(d *doc, p migrate.Plan, opts Options) {
	if len(p.ByHand) == 0 {
		return
	}
	d.heading("By hand")
	d.lead("lo0 migrate leaves these as they are. Each says how to convert it.")
	d.blank()
	for i, f := range p.ByHand {
		label := AreaLabel(f.Area)
		if i > 0 && p.ByHand[i-1].Area == f.Area {
			label = ""
		}
		d.prose(label, opts.paths()(f.Name)+d.t.Muted.Render(separator+f.How))
	}
}

func then(d *doc, p migrate.Plan, opts Options) {
	if len(p.Then) == 0 {
		return
	}
	d.heading("Then")
	pad := strings.Repeat(" ", indent)
	for _, line := range p.Then {
		for i, l := range strings.Split(Wrap(opts.paths()(line), d.width-indent-2), "\n") {
			marker := d.t.Muted.Render("-") + " "
			if i > 0 {
				marker = "  "
			}
			d.lines = append(d.lines, pad+marker+l)
		}
	}
}
