package report

import (
	"slices"
	"strconv"
	"strings"

	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/runtimes"
)

func detailDoc(opts Options) *doc {
	width := opts.Width
	if width <= 0 {
		width = DefaultWidth
	}
	return &doc{t: opts.Theme, width: min(width, MaxWidth)}
}

func (d *doc) title(name, meta string) {
	d.lines = append(d.lines, d.t.Heading.Render(name))
	if meta != "" {
		d.note(meta)
	}
}

// ModelDetail explains one candidate: what the model is, how lo0 would run
// it, where the memory goes and every other way it could run.
func ModelDetail(c match.Candidate, m match.Result, opts Options) string {
	d := detailDoc(opts)
	md := c.Model
	d.title(md.Name, d.join(md.Repo, md.License, md.Released))
	d.blank()
	describeModel(d, md)

	d.blank()
	o := c.Best
	d.row("Recommended", d.join(d.t.Accent.Render(o.Quant.Name), kvLabel(o.KV)+" KV cache"))
	memory(d, o, m.Budget.Bytes)
	if lines := OffloadLines(c, m); lines != nil {
		d.row("RAM", lines[0])
		d.more(lines[1])
		d.more(d.t.Muted.Render(lines[2]))
	}
	d.row("Fits", coverage(d, o.Coverage)+" of your sessions, up to "+format.Tokens(o.MaxContext)+" tokens")
	speed := SpeedLines(c, m)
	d.row("Speed", speed[0])
	d.more(speed[1])
	d.more(d.t.Muted.Render(speed[2]))
	if c.Best.Curve.Source == match.SourcePublished && m.SpeedBasis != "" {
		d.more(d.t.Muted.Render(m.SpeedBasis))
	}
	for i, line := range SpeedComparison(c, m.References, m.ReferenceContext, d.t, d.valueWidth()) {
		if i == 0 {
			d.row("Against", line)
		} else {
			d.more(line)
		}
	}
	if c.Installed != nil {
		have := c.Installed.Option
		d.row("Installed", d.join(c.Installed.Tag+" in "+c.Installed.Runtime.Name(), have.Quant.Name, format.Bytes(uint64(have.Quant.Bytes))))
		if !c.UsesInstalled() {
			d.more(d.t.Muted.Render("lo0 recommends a different build from the one you have"))
		}
	}

	if len(c.Options) > 1 {
		d.blank()
		optionsTable(d, c)
	}
	return d.String()
}

func describeModel(d *doc, md catalog.Model) {
	size := format.Int(int(md.Params/1e9)) + "B parameters"
	if md.Params < 10e9 {
		size = strconv.FormatFloat(float64(md.Params)/1e9, 'f', 1, 64) + "B parameters"
	}
	if md.MoE() {
		size += ", " + strconv.FormatFloat(float64(md.StatedActive())/1e9, 'f', 1, 64) + "B active per token"
	}
	d.row("Model", size)
	d.row("Context", format.Tokens(md.ContextLength)+" tokens")
	d.prose("Attention", attentionText(md.Attention))
	d.prose("Tier", string(md.Tier)+d.t.Muted.Render(" · "+md.TierNote))
	if len(md.Ollama) > 0 {
		d.row("Ollama", strings.Join(md.Ollama, ", "))
	}
}

func attentionText(a catalog.Attention) string {
	perToken := int64(float64(a.FullLayers*a.FullKV) * catalog.KVF16.BytesPerValue())
	parts := []string{format.Plural(a.FullLayers, "layer caches", "layers cache") + " every token, " +
		format.Bytes(uint64(perToken)) + " per token at f16"}
	if a.SlidingLayers > 0 {
		parts = append(parts, format.Plural(a.SlidingLayers, "layer keeps", "layers keep")+" only the last "+format.Tokens(a.Window))
	}
	if a.StateBytes > 0 {
		parts = append(parts, "the rest hold a fixed "+format.Bytes(uint64(a.StateBytes))+" state")
	}
	text := strings.Join(parts, "; ") + "."
	if a.Approximate {
		text += " lo0 cannot model this attention scheme exactly and assumes the worst case."
	}
	return text
}

func memory(d *doc, o match.Option, budget uint64) {
	d.row("Memory", d.join(
		format.Bytes(uint64(o.WeightBytesOnGPU()))+" weights",
		format.Bytes(uint64(o.KVBytes))+" KV",
		format.Bytes(match.RuntimeOverhead)+" runtime",
	))
	d.more(d.t.Muted.Render("= " + format.Bytes(uint64(o.MemoryBytes)) + " of " + format.Bytes(budget) + " available"))
}

// ModelOptions renders the table of every way to run the candidate.
func ModelOptions(c match.Candidate, opts Options) string {
	d := detailDoc(opts)
	optionsTable(d, c)
	return d.String()
}

func optionsTable(d *doc, c match.Candidate) {
	header := []string{"quant", "kv", "window", "fits", "tok/s", "quality"}
	aligns := []align{left, left, right, right, right, right}
	offloads := slices.ContainsFunc(c.Options, match.Option.Offloads)
	if offloads {
		header = append(header, "experts in RAM")
		aligns = append(aligns, right)
	}
	for i, h := range header {
		header[i] = d.t.Muted.Render(h)
	}
	rows := make([][]string, 0, 1+len(c.Options))
	rows = append(rows, header)
	for _, o := range c.Options {
		quant := o.Quant.Name
		if o == c.Best {
			quant = d.t.Accent.Render(quant)
		}
		row := []string{
			quant,
			kvLabel(o.KV),
			format.Tokens(o.MaxContext),
			coverage(d, o.Coverage),
			strconv.Itoa(int(o.TokensPerSecond)),
			format.Percent(o.Quality),
		}
		if offloads {
			row = append(row, OffloadShare(o, c))
		}
		rows = append(rows, row)
	}
	d.table("Options", aligns, rows)
}

func kvLabel(kv catalog.KVType) string {
	if kv == catalog.KVQ8 {
		return "8-bit"
	}
	return "f16"
}

// TooLargeDetail explains why a model does not fit.
func TooLargeDetail(t match.TooLarge, m match.Result, opts Options) string {
	d := detailDoc(opts)
	d.title(t.Model.Name, d.join(t.Model.Repo, t.Model.License, t.Model.Released))
	d.blank()
	describeModel(d, t.Model)
	d.blank()
	d.prose("Needs", d.t.Bad.Render(format.Bytes(uint64(t.NeedsBytes)))+" for its smallest usable build with "+
		format.Tokens(t.NeedsContext)+" of context. This machine has "+BudgetPhrase(m.Budget)+".")
	return d.String()
}

// InstalledDetail describes an installed model that is not in the catalogue.
func InstalledDetail(rm runtimes.Model, opts Options) string {
	d := detailDoc(opts)
	d.title(displayName(rm.Name), rm.Name)
	d.blank()
	if rm.Parameters != "" {
		d.row("Model", rm.Parameters+" parameters")
	}
	if rm.Quantization != "" || rm.SizeBytes > 0 {
		d.row("Build", d.join(rm.Quantization, format.Bytes(rm.SizeBytes)))
	}
	if rm.ContextLength > 0 {
		d.row("Context", format.Tokens(rm.ContextLength)+" tokens")
	}
	tools := d.t.Warn.Render("no tool calling")
	if rm.HasCapability(runtimes.CapabilityTools) {
		tools = "tool calling"
	}
	d.row("Supports", tools)
	d.blank()
	d.prose("", d.t.Muted.Render("Not in the lo0 catalogue, so its KV cache, fit and speed cannot be worked out."))
	return d.String()
}

var statusLabels = map[harness.Status]string{
	harness.StatusNative:    "works as is",
	harness.StatusTranslate: "converted",
	harness.StatusPartial:   "partly",
	harness.StatusLost:      "lost",
}

// StatusLabel is the short, styled name of a feature status.
func StatusLabel(s harness.Status, opts Options) string {
	t := opts.Theme
	label := statusLabels[s]
	switch s {
	case harness.StatusNative:
		return t.Good.Render(label)
	case harness.StatusPartial:
		return t.Warn.Render(label)
	case harness.StatusLost:
		return t.Bad.Render(label)
	default:
		return t.Value.Render(label)
	}
}

// AreaLabel is the display name of a feature area.
func AreaLabel(a harness.Area) string {
	return areaLabels[a]
}

// FeatureDetail explains what happens to one feature in the target harness.
func FeatureDetail(f harness.Feature, target string, opts Options) string {
	d := detailDoc(opts)
	d.title(opts.paths()(f.Name), AreaLabel(f.Area))
	d.blank()
	if f.From != "" {
		d.row("Setup", f.From)
	}
	d.row("In "+target, StatusLabel(f.Status, opts))
	d.prose("How", f.How)
	switch {
	case f.Migrated:
		d.row("Migrate", "lo0 migrate does this for you")
	case f.Status == harness.StatusTranslate || f.Status == harness.StatusPartial:
		d.row("Migrate", d.t.Muted.Render("by hand: lo0 migrate leaves it as it is"))
	}
	if f.Calls > 0 {
		d.row("Used", format.Plural(f.Calls, "call", "calls")+" in your history")
	}
	return d.String()
}
