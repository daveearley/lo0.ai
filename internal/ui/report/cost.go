package report

import (
	"time"

	"lo0.ai/lo0/internal/cost"
	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/ui/draw"
)

const maxModelsInCost = 3

const hoursPerDay = 24

// costSection says what the history is worth at API prices, against the plan
// the user pays for, and what a local model could take on.
func costSection(d *doc, c cost.Report, first time.Time) {
	if c.Tokens.Prompt()+c.Tokens.Output == 0 {
		return
	}
	d.heading("What your Claude Code use is worth")

	since := ""
	if !first.IsZero() {
		since = " since " + first.Local().Format("2 Jan")
	}
	d.row("API value", d.join(
		d.t.Accent.Render(format.USD(c.TotalUSD))+d.t.Muted.Render(since),
		format.USD(c.Last30DaysUSD)+d.t.Muted.Render(" in the last 30 days"),
	))

	switch {
	case c.Plan.MonthlyUSD > 0:
		d.prose("Your plan", c.Plan.Name+" at "+format.USD(c.Plan.MonthlyUSD)+" a month. The last 30 days were worth "+
			d.t.Good.Render(format.Multiple(c.ValueMultiple()))+" that at API prices"+extraUsageNote(c)+".")
	case c.Plan.Name == "API key":
		d.row("Your plan", "Billed per token: this is close to your actual bill.")
	case c.Plan.Name != "unknown":
		d.row("Your plan", c.Plan.Name+d.t.Muted.Render(" · lo0 does not know this plan's price"))
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
		d.prose("Going local", l.Model+" could take "+format.Percent(l.SessionShare)+" of your Claude Code sessions, but only "+
			d.t.Warn.Render(format.Percent(l.Share))+" of their API value ("+format.USD(l.USD)+"). The longest sessions cost the most, and they are the ones a local model cannot hold.")
		if l.OutputHours > 0 {
			d.prose("", "Writing Claude's "+format.Words(c.Tokens.Output)+" output tokens alone would keep it generating for about "+
				hoursText(l.OutputHours)+".")
		}
	}
	d.note("API list prices as of " + longDate(c.PricesAsOf) + " from platform.claude.com. Subagent sessions are included.")
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
