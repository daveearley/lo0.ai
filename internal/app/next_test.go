package app_test

import (
	"strings"
	"testing"
	"time"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/migrate/migratetest"
	"lo0.ai/lo0/internal/runtimes"
	"lo0.ai/lo0/internal/usage"
)

func kinds(steps []app.NextStep) []app.StepKind {
	out := make([]app.StepKind, 0, len(steps))
	for _, s := range steps {
		out = append(out, s.Kind)
	}
	return out
}

func installedClaude(views []app.HarnessView, uses ...harness.Use) []app.HarnessView {
	for i := range views {
		if views[i].ID == harness.ClaudeCode {
			views[i].Installation = harness.Installation{Installed: true, Uses: append([]harness.Use{{Command: "claude", Model: "Claude"}}, uses...)}
		}
	}
	return views
}

// withBestInstalled assesses again with every model that fits downloaded
// in Ollama, so whichever is best is there.
func withBestInstalled(t *testing.T) (*app.Service, app.Assessment) {
	t.Helper()
	_, a := installedService(t)
	res := a.Scan
	for _, c := range a.Match.Candidates {
		tag := "hf.co/" + c.Model.GGUF + ":" + c.Best.Quant.Name
		if len(c.Model.Ollama) > 0 {
			tag = c.Model.Ollama[0]
		}
		res.Runtimes[0].Models = append(res.Runtimes[0].Models, runtimes.Model{Name: tag, SizeBytes: 20 << 30, Quantization: c.Best.Quant.Name})
	}
	svc, a := assess(t, res, migrator(migratetest.NewFiles(nil)))
	if best, _ := a.Match.Best(); best.Installed == nil {
		t.Fatalf("the best fit, %s, is still not installed", best.Model.Name)
	}
	return svc, a
}

func TestNextDownloadsABestFitThatIsNotThere(t *testing.T) {
	svc, a := installedService(t)
	inv, _ := svc.Inventory(a)
	o := app.Next(a, inv, installedClaude(svc.Harnesses(t.Context(), a)))
	if got := kinds(o.Steps); got[0] != app.StepDownload || !strings.Contains(o.Headline, "download") {
		t.Errorf("steps %v, headline %q; want a download first", got, o.Headline)
	}
}

func TestNextTriesTheDownloadedBestFit(t *testing.T) {
	svc, a := withBestInstalled(t)
	inv, _ := svc.Inventory(a)
	views := installedClaude(svc.Harnesses(t.Context(), a))
	best, _ := a.Match.Best()
	o := app.Next(a, inv, views)
	if got := kinds(o.Steps); len(got) < 2 || got[0] != app.StepTry || got[1] != app.StepKeep || got[len(got)-1] != app.StepBrowse {
		t.Fatalf("steps = %v, want try, keep, ..., browse", got)
	}
	if o.Steps[0].Harness != harness.ClaudeCode || o.Steps[0].Label != "Try it in Claude Code for one session" {
		t.Errorf("try = %+v", o.Steps[0])
	}
	if best.Installed != nil && !strings.Contains(o.Headline, "is already downloaded") {
		t.Errorf("headline = %q", o.Headline)
	}
}

func TestNextKeepsWhatWasTriedAndKnowsWhatIsKept(t *testing.T) {
	svc, a := withBestInstalled(t)
	best, _ := a.Match.Best()
	a.Scan.Usage.Records = append(a.Scan.Usage.Records, usage.SessionRecord{
		Start: time.Date(2026, 11, 5, 8, 0, 0, 0, time.UTC),
		Usage: []usage.ModelUsage{{Model: "lo0/" + best.Model.ID + ":64k"}},
	})
	inv, _ := svc.Inventory(a)
	views := installedClaude(svc.Harnesses(t.Context(), a))
	o := app.Next(a, inv, views)
	if o.LastTryModel != best.Model.Name || o.LastTry.IsZero() || o.Steps[0].Kind != app.StepKeep {
		t.Errorf("last try %q at %v, steps %v; want keep first after a try", o.LastTryModel, o.LastTry, kinds(o.Steps))
	}

	views = installedClaude(svc.Harnesses(t.Context(), a), harness.Use{Command: "claude-local", Model: best.Model.Name + " through Ollama", Local: true})
	o = app.Next(a, inv, views)
	if o.Kept == "" || o.Steps[0].Kind == app.StepTry || o.Steps[0].Kind == app.StepKeep || !strings.Contains(o.Headline, "is set up as your local model") {
		t.Errorf("kept %q, steps %v, headline %q", o.Kept, kinds(o.Steps), o.Headline)
	}
}

func TestNextInstallsAMissingHarnessFirst(t *testing.T) {
	svc, a := withBestInstalled(t)
	inv, _ := svc.Inventory(a)
	o := app.Next(a, inv, svc.Harnesses(t.Context(), a))
	if o.Steps[0].Kind != app.StepInstall || o.Steps[0].Harness != harness.ClaudeCode {
		t.Errorf("steps = %v, want installing the recommended harness first", kinds(o.Steps))
	}
}
