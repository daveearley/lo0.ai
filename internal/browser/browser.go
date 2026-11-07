// Package browser is lo0's front end for a web page, compiled to
// WebAssembly. It assesses hardware the visitor names against the embedded
// catalogue; it cannot see their machine, history or setup.
package browser

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/ui/report"
	"lo0.ai/lo0/internal/ui/theme"
	"lo0.ai/lo0/internal/wiring"
)

// Request names the hardware to assess. Sizes are in GB.
type Request struct {
	GPU    string `json:"gpu,omitzero"`
	VRAM   int    `json:"vram,omitzero"`
	GPUs   int    `json:"gpus,omitzero"`
	RAM    int    `json:"ram,omitzero"`
	Mac    string `json:"mac,omitzero"`
	Memory int    `json:"memory,omitzero"`
	// Width is the report's width in columns, never fewer than the
	// terminal default, below which model names are cut short.
	Width int `json:"width,omitzero"`
}

// Answer is what lo0 concluded about the hardware, in the report's words.
type Answer struct {
	// Reasons are the verdict's reasons, which without history describe the
	// best fit for the hardware.
	Reasons []string `json:"reasons"`
	// Models is the report's models section as plain text.
	Models string `json:"models"`
	// Command assesses the same hardware with the visitor's own history.
	Command string `json:"command"`
}

// Hardware is everything a page can offer to assess.
type Hardware struct {
	GPUs []match.KnownGPU `json:"gpus"`
	Macs []match.KnownMac `json:"macs"`
}

// Known lists the hardware lo0 has figures for.
func Known() Hardware {
	return Hardware{GPUs: match.KnownGPUs(), Macs: match.KnownMacs()}
}

// Assess matches the named hardware against the catalogue.
func Assess(ctx context.Context, r Request) (Answer, error) {
	q := r.query()
	sim, err := match.Simulate(q)
	if err != nil {
		return Answer{}, err
	}
	svc, err := wiring.HardwareOnly(sim)
	if err != nil {
		return Answer{}, err
	}
	a, err := svc.Assess(ctx, nil)
	if err != nil {
		return Answer{}, err
	}
	opts := report.Options{Width: max(r.Width, report.DefaultWidth), Theme: theme.Plain(), Titled: true}
	return Answer{
		Reasons: slices.Clone(a.Match.Verdict.Reasons),
		Models:  strings.TrimLeft(report.RenderSection(report.SectionModels, a, opts), "\n"),
		Command: Command(q),
	}, nil
}

func (r Request) query() match.HardwareQuery {
	return match.HardwareQuery{GPU: r.GPU, VRAM: r.VRAM, GPUs: r.GPUs, RAM: r.RAM, Mac: r.Mac, Memory: r.Memory}
}

// Command is the lo0 match command that assesses q, leaving out sizes the
// hardware's name already settles.
func Command(q match.HardwareQuery) string {
	args := []string{"lo0", "match"}
	if q.Mac != "" {
		args = append(args, "--mac", quote(q.Mac))
		if q.Memory > 0 {
			args = append(args, "--memory", strconv.Itoa(q.Memory))
		}
		return strings.Join(args, " ")
	}
	args = append(args, "--gpu", quote(q.GPU))
	if q.VRAM > 0 && soldInSizes(q.GPU) {
		args = append(args, "--vram", strconv.Itoa(q.VRAM))
	}
	if q.GPUs > 1 {
		args = append(args, "--gpus", strconv.Itoa(q.GPUs))
	}
	if q.RAM > 0 {
		args = append(args, "--ram", strconv.Itoa(q.RAM))
	}
	return strings.Join(args, " ")
}

func soldInSizes(gpu string) bool {
	i := slices.IndexFunc(match.KnownGPUs(), func(g match.KnownGPU) bool { return strings.EqualFold(g.Name, gpu) })
	return i < 0 || len(match.KnownGPUs()[i].VRAM) > 1
}

func quote(s string) string {
	if strings.ContainsAny(s, " \t") {
		return `"` + s + `"`
	}
	return s
}
