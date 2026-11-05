package report_test

import (
	"strings"
	"testing"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/redact"
	"lo0.ai/lo0/internal/scan"
	"lo0.ai/lo0/internal/ui/report"
)

// A warning is wrapped across lines, so a long path or one with spaces
// must be hidden before wrapping or --redact misses the broken pieces.
func TestWarningsAreHiddenBeforeWrapping(t *testing.T) {
	home := "/Users/Jonathan SECRET_Applebaum-Smith"
	project := home + "/code/SECRET_client-acme"
	hide := func(s string) string { return redact.Paths(s, home, []string{project}) }
	res := scan.Result{Warnings: []scan.Warning{
		{Stage: "setup", Message: "reading " + home + "/.claude/settings.json: open settings.json: permission denied"},
		{Stage: "history", Message: "skipped a malformed line in " + project + "/.mcp.json, which lo0 could not read"},
	}}
	for name, render := range map[string]func() string{
		"scan": func() string { return report.Render(res, report.Options{Width: 80, Home: home, Hide: hide}) },
		"match": func() string {
			return report.RenderSection(report.SectionWarnings, app.Assessment{Scan: res}, report.Options{Width: 80, Home: home, Hide: hide})
		},
	} {
		t.Run(name, func(t *testing.T) {
			if out := render(); strings.Contains(out, "SECRET_") {
				t.Errorf("redacted warnings still name the user or project:\n%s", out)
			}
		})
	}
}
