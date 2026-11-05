package tui

import (
	"math"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/ui/report"
	"lo0.ai/lo0/internal/ui/theme"
)

// labs names the organisation behind a Hugging Face repository owner.
var labs = map[string]string{
	"Qwen": "Qwen", "openai": "OpenAI", "mistralai": "Mistral", "zai-org": "Z.ai",
	"MiniMaxAI": "MiniMax", "moonshotai": "Moonshot AI", "deepseek-ai": "DeepSeek",
	"google": "Google", "nvidia": "NVIDIA", "ibm-granite": "IBM",
}

func lab(repo string) string {
	owner, _, _ := strings.Cut(repo, "/")
	if name, ok := labs[owner]; ok {
		return name
	}
	return owner
}

var licences = map[string]string{
	"apache-2.0": "Apache 2.0", "mit": "MIT", "other": "custom licence",
	"gemma": "Gemma licence", "llama3": "Llama licence",
}

func licenceName(id string) string {
	if name, ok := licences[strings.ToLower(id)]; ok {
		return name
	}
	return id
}

func released(date string) string {
	t, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return date
	}
	return t.Format("Jan 2006")
}

const labelColumn = 11

// detail renders the panel for one model. full adds every quantization
// option and the tier explanation.
func (s models) detail(r modelRow, width int, full bool) string {
	t := s.env.theme
	switch {
	case r.unknown != nil:
		return report.InstalledDetail(*r.unknown, s.env.reportOptions(width))
	case r.tooLarge != nil:
		return s.profileView(r.tooLarge.Model, nil, width, full) + "\n" +
			section(t, "ON THIS MACHINE") +
			t.Bad.Render("Too large. ") + t.Value.Render(report.Wrap("Its smallest usable build needs "+
			format.Bytes(uint64(r.tooLarge.NeedsBytes))+" with "+format.Tokens(r.tooLarge.NeedsContext)+" of context; this machine has "+
			format.Bytes(s.env.assessment.Match.Budget.Bytes)+" for models.", width))
	}
	c := r.candidate
	var b strings.Builder
	b.WriteString(s.profileView(c.Model, c, width, full))
	b.WriteString("\n" + section(t, "ON THIS MACHINE"))
	b.WriteString(s.machineView(*c, width))
	if full {
		b.WriteString("\n" + section(t, "EVERY WAY TO RUN IT"))
		b.WriteString(report.ModelOptions(*c, s.env.reportOptions(width)))
		b.WriteString("\n" + section(t, "TIER "+string(c.Model.Tier)))
		b.WriteString(t.Muted.Render(report.Wrap(c.Model.TierNote, width)) + "\n")
	}
	return b.String()
}

func section(t theme.Theme, label string) string {
	return t.Label.Render(label) + "\n"
}

// profileView is what the lab says: title line, summary, good at and watch
// out for, and published scores.
func (s models) profileView(m catalog.Model, c *match.Candidate, width int, full bool) string {
	t := s.env.theme
	meta := []string{lab(m.Repo)}
	if m.License != "" {
		meta = append(meta, licenceName(m.License))
	}
	if m.Released != "" {
		meta = append(meta, released(m.Released))
	}
	status := ""
	if c != nil && c.Installed != nil {
		status = t.Good.Render("  ✓ installed")
	}
	title := t.Heading.Render(m.Name) + status
	right := t.Muted.Render(strings.Join(meta, " · "))
	gap := width - lipgloss.Width(title) - lipgloss.Width(right)
	var b strings.Builder
	if gap >= 2 {
		b.WriteString(title + strings.Repeat(" ", gap) + right + "\n")
	} else {
		b.WriteString(title + "\n" + right + "\n")
	}
	if p := m.Profile; p.Summary != "" {
		b.WriteString(t.Value.Render(report.Wrap(p.Summary, width)) + "\n")
	}
	b.WriteString("\n" + s.claims(m.Profile, width))
	if len(m.Profile.Benchmarks) > 0 {
		scores := make([]string, 0, len(m.Profile.Benchmarks))
		for _, bm := range m.Profile.Benchmarks {
			scores = append(scores, t.Muted.Render(bm.Label()+" ")+t.Value.Render(bm.Score))
		}
		b.WriteString("\n" + t.Label.Render("SCORES") + "  " + atoms(scores, t.Muted.Render("  ·  "), width-8, "        ") + "\n")
	}
	if full && m.Profile.Card != "" {
		b.WriteString(t.Muted.Render("From the model card: "+m.Profile.Card) + "\n")
	}
	return b.String()
}

// claims lays out the lab's good-at and watch-out lines side by side, or
// stacked on narrow screens.
func (s models) claims(p catalog.Profile, width int) string {
	t := s.env.theme
	list := func(label, mark string, items []string, style lipgloss.Style, w int) string {
		lines := []string{t.Label.Render(label)}
		if len(items) == 0 {
			lines = append(lines, t.Muted.Render("Nothing stated on the model card."))
		}
		for _, it := range items {
			for i, l := range strings.Split(report.Wrap(it, w-2), "\n") {
				prefix := style.Render(mark) + " "
				if i > 0 {
					prefix = "  "
				}
				lines = append(lines, prefix+t.Value.Render(l))
			}
		}
		return strings.Join(lines, "\n")
	}
	if width < 70 {
		return list("GOOD AT", "+", p.GoodAt, t.Good, width) + "\n\n" + list("WATCH OUT FOR", "–", p.WatchOut, t.Warn, width) + "\n"
	}
	col := (width - 4) / 2
	left := lipgloss.NewStyle().Width(col).Render(list("GOOD AT", "+", p.GoodAt, t.Good, col))
	right := list("WATCH OUT FOR", "–", p.WatchOut, t.Warn, col)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, "    ", right) + "\n"
}

// machineView is lo0's own analysis: memory, session fit, speed and the
// insights it drew from them.
func (s models) machineView(c match.Candidate, width int) string {
	t := s.env.theme
	m := s.env.assessment.Match
	ctx := s.env.assessment.Scan.Usage.Context
	o := c.Best
	cells := min(max(width-labelColumn-24, 16), 48)
	label := func(l string) string { return t.Label.Render(padRight(l, labelColumn)) }

	var b strings.Builder
	mem, legend := memoryChart(t, o, m.Budget.Bytes, cells)
	b.WriteString(label("Memory") + mem + "  " + t.Value.Render(format.Bytes(uint64(o.MemoryBytes))) +
		t.Muted.Render(" of "+format.Bytes(m.Budget.Bytes)) + "\n")
	b.WriteString(strings.Repeat(" ", labelColumn) + legend + "\n")

	if len(ctx.Peaks) > 0 {
		chart, marker := sessionChart(t, ctx, o.MaxContext, cells)
		b.WriteString(label("Sessions") + chart + "  " + coverageStyle(t, o.Coverage).Render(format.Percent(o.Coverage)) +
			t.Muted.Render(" fit") + "\n")
		b.WriteString(strings.Repeat(" ", labelColumn) + t.Muted.Render(marker) + "\n")
	}

	points := report.SpeedPoints(o, m.ReferenceContext)
	writes := make([]string, 0, len(points))
	reads := make([]string, 0, len(points))
	for _, tokens := range points {
		writes = append(writes, t.Muted.Render(format.Tokens(tokens)+" ")+t.Value.Render(strconv.Itoa(int(m.SpeedAt(c, tokens)))))
		reads = append(reads, t.Muted.Render(format.Tokens(tokens)+" ")+t.Value.Render(format.Int(int(m.ReadSpeedAt(c, tokens)))))
	}
	b.WriteString(label("Writing") + strings.Join(writes, t.Muted.Render("   ")) + t.Muted.Render("  tok/s") + "\n")
	b.WriteString(label("Reading") + strings.Join(reads, t.Muted.Render("   ")) + t.Muted.Render("  tok/s, new tokens at that context") + "\n")
	b.WriteString(strings.Repeat(" ", labelColumn) + t.Muted.Render(report.SourceLabel(o.Curve)) + "\n")
	for i, line := range report.SpeedComparison(c, m.References, m.ReferenceContext, t, width-labelColumn) {
		if i == 0 {
			b.WriteString(label("Against") + line + "\n")
		} else {
			b.WriteString(strings.Repeat(" ", labelColumn) + line + "\n")
		}
	}
	b.WriteString(label("Build") + t.Value.Render(o.Quant.Name) + t.Muted.Render(" · "+kvName(o.KV)+" KV cache · keeps "+format.Percent(o.Quality)+" of full quality") + "\n")

	if len(c.Insights.Strengths)+len(c.Insights.Concerns) > 0 {
		b.WriteString("\n")
	}
	for _, line := range c.Insights.Strengths {
		b.WriteString(t.Good.Render("✓ ") + t.Value.Render(line) + "\n")
	}
	for _, line := range c.Insights.Concerns {
		b.WriteString(t.Warn.Render("! ") + t.Value.Render(line) + "\n")
	}
	return b.String()
}

func kvName(kv catalog.KVType) string {
	if kv == catalog.KVQ8 {
		return "8-bit"
	}
	return "f16"
}

// compareView sets two to four models side by side: the marked ones, or the
// three best.
func (s models) compareView(width int) string {
	t := s.env.theme
	m := s.env.assessment.Match
	var picks []match.Candidate
	for _, id := range s.marked {
		for _, c := range m.Candidates {
			if c.Model.ID == id {
				picks = append(picks, c)
			}
		}
	}
	if len(picks) < 2 {
		picks = m.Candidates[:min(3, len(m.Candidates))]
	}
	if len(picks) == 0 {
		return t.Muted.Render("Nothing fits this machine, so there is nothing to compare.")
	}
	const labelW = 20
	col := max((width-labelW)/len(picks), 14)

	type metric struct {
		label string
		value func(match.Candidate) (string, float64)
		// better says which direction wins; zero means no winner.
		better int
	}
	metrics := []metric{
		{"Sessions that fit", func(c match.Candidate) (string, float64) {
			return format.Percent(c.Best.Coverage), c.Best.Coverage
		}, 1},
		{"Speed at " + format.Tokens(m.ReferenceContext), func(c match.Candidate) (string, float64) {
			return strconv.Itoa(int(c.Best.TokensPerSecond)) + " tok/s", c.Best.TokensPerSecond
		}, 1},
		{"Context window", func(c match.Candidate) (string, float64) {
			return format.Tokens(c.Best.MaxContext), float64(c.Best.MaxContext)
		}, 1},
		{"Memory", func(c match.Candidate) (string, float64) {
			return format.Bytes(uint64(c.Best.MemoryBytes)), float64(c.Best.MemoryBytes)
		}, -1},
		{"Quality kept", func(c match.Candidate) (string, float64) {
			return format.Percent(c.Best.Quality) + " " + c.Best.Quant.Name, c.Best.Quality
		}, 1},
		{"Tier", func(c match.Candidate) (string, float64) {
			return string(c.Model.Tier), -float64(c.Model.Tier.Rank())
		}, 1},
	}
	models := make([]catalog.Model, 0, len(picks))
	for _, c := range picks {
		models = append(models, c.Model)
	}
	shared := catalog.SharedBenchmarks(models)
	for _, key := range shared {
		var name string
		for _, m := range models {
			if b, ok := m.Benchmark(key); ok {
				name = b.Name
				break
			}
		}
		metrics = append(metrics, metric{ansi.Truncate(name, labelW-2, "…"), func(c match.Candidate) (string, float64) {
			b, ok := c.Model.Benchmark(key)
			v, isNumber := b.Value()
			if !ok || !isNumber {
				return "–", math.Inf(-1)
			}
			if b.Setting != "" {
				return b.Score + " " + b.Setting, v
			}
			return b.Score, v
		}, 1})
	}
	metrics = append(metrics, []metric{
		{"Installed", func(c match.Candidate) (string, float64) {
			if c.Installed != nil {
				return "✓ " + c.Installed.Tag, 1
			}
			return "no", 0
		}, 0},
		{"Licence", func(c match.Candidate) (string, float64) { return licenceName(c.Model.License), 0 }, 0},
	}...)

	var b strings.Builder
	b.WriteString(strings.Repeat(" ", labelW))
	for _, c := range picks {
		b.WriteString(padRight(ansi.Truncate(t.Heading.Render(c.Model.Name), col-2, "…"), col))
	}
	b.WriteString("\n\n")
	for _, mt := range metrics {
		b.WriteString(t.Label.Render(padRight(mt.label, labelW)))
		best := 0.0
		for i, c := range picks {
			_, v := mt.value(c)
			if i == 0 || float64(mt.better)*v > float64(mt.better)*best {
				best = v
			}
		}
		for _, c := range picks {
			text, v := mt.value(c)
			style := t.Value
			if mt.better != 0 && v == best && len(picks) > 1 {
				style = t.Good
			}
			b.WriteString(padRight(ansi.Truncate(style.Render(text), col-2, "…"), col))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n" + t.Label.Render("WHAT LO0 FOUND") + "\n")
	for _, c := range picks {
		b.WriteString(t.Heading.Render(c.Model.Name) + "\n")
		for _, l := range c.Insights.Strengths {
			b.WriteString("  " + t.Good.Render("✓ ") + t.Value.Render(l) + "\n")
		}
		for _, l := range c.Insights.Concerns {
			b.WriteString("  " + t.Warn.Render("! ") + t.Value.Render(l) + "\n")
		}
	}
	note := "Green marks the best value in each row. Speeds are estimates."
	if len(shared) > 0 {
		note += " Scores are each lab's own, from its model card."
	}
	b.WriteString("\n" + t.Muted.Render(report.Wrap(note, width)) + "\n")
	return b.String()
}
