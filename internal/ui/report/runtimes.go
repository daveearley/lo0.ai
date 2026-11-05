package report

import (
	"path"
	"strings"

	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/runtimes"
)

const maxModelsShown = 8

func runtimesSection(d *doc, rts []runtimes.Runtime) {
	d.heading("Local runtimes")
	for _, rt := range rts {
		switch {
		case !rt.Installed:
			d.row(rt.Name, d.t.Muted.Render("not installed"))
			continue
		case rt.Running:
			d.row(rt.Name, d.join(rt.Version, d.t.Good.Render("running")+d.t.Muted.Render(" on "+hostOf(rt.Endpoint)), modelCount(d, rt.Models)))
		case rt.Service:
			d.row(rt.Name, d.join(rt.Version, d.t.Muted.Render("not running"), modelCount(d, rt.Models)))
		default:
			d.row(rt.Name, d.join(rt.Version, "installed", modelCount(d, rt.Models)))
		}
		modelTable(d, rt.Models)
	}
}

// modelTable lists models with the facts that decide agentic fit. Vision is
// left out: it does not affect whether a model can drive a coding harness.
func modelTable(d *doc, models []runtimes.Model) {
	shown := models[:min(len(models), maxModelsShown)]
	rows := make([][]string, 0, len(shown))
	for _, m := range shown {
		ctx := ""
		if m.ContextLength > 0 {
			ctx = format.Tokens(m.ContextLength)
		}
		size := ""
		if m.SizeBytes > 0 {
			size = format.Bytes(m.SizeBytes)
		}
		var caps []string
		for _, c := range []runtimes.Capability{runtimes.CapabilityTools, runtimes.CapabilityThinking} {
			if m.HasCapability(c) {
				caps = append(caps, string(c))
			}
		}
		capText := d.t.Muted.Render(strings.Join(caps, " "))
		if !m.HasCapability(runtimes.CapabilityTools) {
			capText = d.t.Warn.Render("no tool calling")
		}
		rows = append(rows, []string{displayName(m.Name), m.Parameters, m.Quantization, size, ctx, capText})
	}
	d.table("", []align{left, right, left, right, right, left}, rows)
	if extra := len(models) - len(shown); extra > 0 {
		d.more(d.t.Muted.Render("and " + format.Plural(extra, "more model", "more models")))
	}
}

func hostOf(endpoint string) string {
	_, host, ok := strings.Cut(endpoint, "://")
	if !ok {
		return endpoint
	}
	return host
}

func modelCount(d *doc, models []runtimes.Model) string {
	if len(models) == 0 {
		return d.t.Muted.Render("no models")
	}
	return format.Plural(len(models), "model", "models")
}

// displayName drops registry and owner prefixes such as "hf.co/owner/" and
// Ollama's implicit ":latest" tag, which make names long without helping
// anyone recognise the model.
func displayName(name string) string {
	return strings.TrimSuffix(path.Base(name), ":latest")
}
