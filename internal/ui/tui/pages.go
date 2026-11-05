package tui

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/ui/report"
)

// page is a scrollable screen of rendered report sections.
type page struct {
	env    env
	render func(width int) string
	vp     viewport.Model
}

func newPage(e env, render func(width int) string) screen {
	return page{env: e, render: render, vp: viewport.New()}
}

func optional(s string) string {
	if s == "" {
		return ""
	}
	return "\n" + s
}

func (p page) update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case sizeMsg:
		p.vp.SetWidth(msg.width)
		p.vp.SetHeight(msg.height)
		p.vp.SetContent(p.render(msg.width))
		return p, nil
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, p.env.keys.Top):
			p.vp.GotoTop()
			return p, nil
		case key.Matches(msg, p.env.keys.Bottom):
			p.vp.GotoBottom()
			return p, nil
		}
	}
	var cmd tea.Cmd
	p.vp, cmd = p.vp.Update(msg)
	return p, cmd
}

func (p page) view(int, int) string { return p.vp.View() }

func (p page) hints() []hint {
	if p.vp.TotalLineCount() > p.vp.Height() {
		where := format.Percent(p.vp.ScrollPercent())
		if p.vp.AtBottom() {
			where = "end"
		}
		return []hint{{"↑↓", "scroll · " + where}, {"tab", "next screen"}}
	}
	return []hint{{"tab", "next screen"}}
}

func (page) capturesKeys() bool { return false }

func featureEntries(e env, target match.TargetFit) []entry {
	short := func(p string) string { return shortPath(e.home, p) }
	out := make([]entry, 0, len(target.Features))
	for _, area := range harness.Areas {
		for _, f := range target.Features {
			if f.Area != area {
				continue
			}
			title := f.Name
			if area == harness.AreaInstructions || area == harness.AreaPermissions {
				title = short(f.Name)
			}
			meta := report.StatusLabel(f.Status, e.reportOptions(0))
			if f.Calls > 0 {
				meta = e.theme.Muted.Render(format.Int(f.Calls)+" calls  ") + meta
			}
			out = append(out, entry{
				group:  report.AreaLabel(area),
				title:  title,
				meta:   meta,
				search: fmt.Sprintf("%s %s %s %s", f.Name, area, f.Status, f.How),
				detail: func(w int) string { return report.FeatureDetail(f, target.Name, e.reportOptions(w)) },
			})
		}
	}
	return out
}
