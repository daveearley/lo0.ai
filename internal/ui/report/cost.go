package report

import (
	"net/url"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"lo0.ai/lo0/internal/cost"
	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/ui/draw"
)

const maxModelsInCost = 3

const hoursPerDay = 24

// mostOfTheValue is how close the share of API value a local model could
// take must come to its share of sessions to be called most of it.
const mostOfTheValue = 0.9

// costSection says what each harness's history is worth at API prices,
// against the plan it runs on, and what a local model could take on, then
// what they come to together.
func costSection(d *doc, s cost.Summary) {
	for _, c := range s.Harnesses {
		harnessCost(d, c)
	}
	if len(s.Harnesses) < 2 {
		return
	}
	d.heading(Harnesses(s) + " together")
	apiValue(d, s.TotalUSD, s.Last30DaysUSD, firstOf(s))
}

// apiValue writes the total and the last 30 days on one line, or on two
// when one would not fit.
func apiValue(d *doc, total, last30 float64, first time.Time) {
	all := d.t.Accent.Render(format.USD(total)) + d.t.Muted.Render(since(first))
	recent := format.USD(last30) + d.t.Muted.Render(" in the last 30 days")
	if line := d.join(all, recent); lipgloss.Width(line) <= d.valueWidth() {
		d.row("API value", line)
		return
	}
	d.row("API value", all)
	d.more(recent)
}

// Harnesses names the harnesses a summary covers, such as "Claude Code and
// Codex".
func Harnesses(s cost.Summary) string {
	names := make([]string, len(s.Harnesses))
	for i, c := range s.Harnesses {
		names[i] = c.Harness
	}
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	default:
		return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
	}
}

// CostCovers says which harnesses a summary's totals cover, for a line under
// them.
func CostCovers(s cost.Summary) string {
	if len(s.Harnesses) > 1 {
		return "across " + Harnesses(s)
	}
	return "in " + Harnesses(s)
}

func firstOf(s cost.Summary) time.Time {
	var first time.Time
	for _, c := range s.Harnesses {
		if !c.First.IsZero() && (first.IsZero() || c.First.Before(first)) {
			first = c.First
		}
	}
	return first
}

func since(first time.Time) string {
	if first.IsZero() {
		return ""
	}
	return " since " + first.Local().Format("2 Jan")
}

func harnessCost(d *doc, c cost.Report) {
	d.heading("What your " + c.Harness + " use is worth")
	apiValue(d, c.TotalUSD, c.Last30DaysUSD, c.First)

	switch p := c.Plan; {
	case p.MonthlyUSD > 0:
		d.prose("Your plan", p.Name+" at "+format.USD(p.MonthlyUSD)+" a month. The last 30 days were worth "+
			d.t.Good.Render(format.Multiple(c.ValueMultiple()))+" that at API prices"+extraUsageNote(c)+".")
	case p.FromUSD > 0:
		d.prose("Your plan", p.Name+", from "+format.USD(p.FromUSD)+" a month. The last 30 days were worth "+
			d.t.Good.Render(format.Multiple(c.ValueMultiple()))+" its cheapest tier at API prices.")
	case p.PerToken:
		d.row("Your plan", "Billed per token: this is close to your actual bill.")
	case p.Known:
		d.row("Your plan", p.Name+d.t.Muted.Render(" · no monthly price"))
	case p.Name != "unknown":
		d.row("Your plan", p.Name+d.t.Muted.Render(" · lo0 does not know this plan's price"))
	}

	d.prose("Prompt cache", format.Percent(c.Cache.ReadShare)+" of your input was read from the cache, which saved "+
		format.USD(c.Cache.SavedUSD)+" over sending it fresh.")
	d.row("Tokens", d.join(format.Words(c.Tokens.Prompt())+" read", format.Words(c.Tokens.Output)+" written"))

	var models []string
	for _, m := range c.ByModel {
		if !m.Priced || m.USD < 1 {
			continue
		}
		models = append(models, format.Model(m.Model)+" "+d.t.Muted.Render(format.USD(m.USD)))
	}
	if len(models) > maxModelsInCost {
		models = append(models[:maxModelsInCost], d.t.Muted.Render("+"+format.Int(len(models)-maxModelsInCost)+" more"))
	}
	if len(models) > 0 {
		d.row("By model", d.join(models...))
	}
	byMonth(d, c.ByMonth)

	if l := c.Local; l.Model != "" {
		sessions := l.Model + " could take " + format.Percent(l.SessionShare) + " of your " + c.Harness + " sessions"
		if l.Share < l.SessionShare*mostOfTheValue {
			d.prose("Going local", sessions+", but only "+d.t.Warn.Render(format.Percent(l.Share))+" of their API value ("+format.USD(l.USD)+
				"). The longest sessions cost the most, and they are the ones a local model cannot hold.")
		} else {
			d.prose("Going local", sessions+" and "+d.t.Good.Render(format.Percent(l.Share))+" of their API value ("+format.USD(l.USD)+").")
		}
		if l.OutputHours > 0 {
			d.prose("", "Writing the "+format.Words(c.Tokens.Output)+" output tokens in your "+c.Harness+" history alone would keep it generating for about "+
				hoursText(l.OutputHours)+".")
		}
	}
	d.note(strings.TrimSpace(pricesNote(c.Prices) + " Subagent sessions are included."))
}

// pricesNote says whose list prices the figures use, from where and when.
func pricesNote(lists []cost.PriceList) string {
	if len(lists) == 0 {
		return ""
	}
	parts := make([]string, len(lists))
	for i, l := range lists {
		host := l.Source
		if u, err := url.Parse(l.Source); err == nil && u.Host != "" {
			host = u.Host
		}
		parts[i] = l.Provider + " API list prices as of " + longDate(l.AsOf) + " from " + host
	}
	return strings.Join(parts, "; ") + "."
}

func longDate(iso string) string {
	t, err := time.Parse(time.DateOnly, iso)
	if err != nil {
		return iso
	}
	return t.Format("2 Jan 2006")
}

func extraUsageNote(c cost.Report) string {
	if c.ExtraUsage {
		return "; extra usage is on, so anything beyond the plan is billed on top"
	}
	return ""
}

func hoursText(h float64) string {
	if h < hoursPerDay*2 {
		return format.Int(int(h+0.5)) + " hours"
	}
	return format.Int(int(h+0.5)) + " hours (" + format.Int(int(h/hoursPerDay+0.5)) + " days) non-stop"
}

// monthBarCells is the width of the bars in the monthly breakdown.
const monthBarCells = 24

// byMonth draws the API value of each month as a bar, so a rising bill is
// visible at a glance.
func byMonth(d *doc, months []cost.MonthCost) {
	if len(months) < 2 {
		return
	}
	top := 0.0
	for _, m := range months {
		top = max(top, m.USD)
	}
	if top <= 0 {
		return
	}
	rows := make([][]string, 0, len(months))
	for _, m := range months {
		filled, _ := draw.Bar(m.USD/top, monthBarCells)
		rows = append(rows, []string{monthLabel(m.Month), format.USD(m.USD), d.t.Accent.Render(filled)})
	}
	d.table("By month", []align{left, right, left}, rows)
}

// monthLabel turns "2026-10" into "Oct 2026".
func monthLabel(m string) string {
	t, err := time.Parse("2006-01", m)
	if err != nil {
		return m
	}
	return t.Format("Jan 2006")
}
