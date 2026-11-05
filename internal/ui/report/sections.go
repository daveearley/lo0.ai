package report

import (
	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/claudecode"
)

// Section names one part of a report that front ends can show on its own.
type Section int

// Report sections.
const (
	SectionVerdict Section = iota
	SectionCarryover
	SectionMachine
	SectionSetup
	SectionHistory
	SectionWarnings
	SectionCost
	SectionProjects
	// SectionHardware is the machine without its runtimes, for a front end
	// that shows the runtimes its own way.
	SectionHardware
)

// RenderSection renders a single section without the report header, for
// front ends that lay sections out themselves. Scan-only sections need only
// a.Scan.
func RenderSection(s Section, a app.Assessment, opts Options) string {
	res, m := a.Scan, a.Match
	width := opts.Width
	if width <= 0 {
		width = DefaultWidth
	}
	d := &doc{t: opts.Theme, width: min(width, MaxWidth)}
	paths := opts.paths()
	switch s {
	case SectionVerdict:
		verdictSection(d, m.Verdict)
	case SectionCarryover:
		carryoverSection(d, m)
	case SectionMachine:
		machineSection(d, res.Machine)
		runtimesSection(d, res.Runtimes)
	case SectionSetup:
		setupSection(d, res.Setup, res.Claude, res.Usage.Of(claudecode.Harness).Format, paths)
	case SectionHistory:
		historySection(d, res.Usage, res.Layout.ProjectsDir(), paths)
	case SectionWarnings:
		warningsSection(d, res.Warnings, opts.Hide)
	case SectionCost:
		costSection(d, a.Cost, res.Usage.First)
	case SectionProjects:
		projectsSection(d, m, paths)
	case SectionHardware:
		machineSection(d, res.Machine)
	}
	if len(d.lines) == 0 {
		return ""
	}
	return d.String()
}
