package cli

import (
	"context"
	"errors"
	"fmt"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/ui/report"
)

func (a *App) benchmark(ctx context.Context, args []string) error {
	if err := a.refuseHardware("benchmark", args); err != nil {
		return err
	}
	var c commonFlags
	var model string
	var yes bool
	fs := a.flags("benchmark", "[--model ID] [--yes]")
	fs.StringVar(&model, "model", "", "a downloaded catalogue model `ID`; the best fit by default")
	fs.BoolVar(&yes, "yes", false, "measure without asking")
	fs.StringVar(&c.claudeDir, "claude-dir", "", "read Claude Code files from `DIR` instead of ~/.claude")
	fs.StringVar(&c.codexDir, "codex-dir", "", "read Codex files from `DIR` instead of ~/.codex")
	fs.BoolVar(&c.offline, "offline", false, "do not look up the newest Claude Code release")
	if err := parse(fs, args); err != nil {
		return err
	}
	svc, err := a.service(c)
	if err != nil {
		return err
	}
	assessment, err := observed(ctx, a, true, svc.Assess)
	if err != nil {
		return err
	}
	if model == "" {
		model = installedBest(assessment)
	}
	if model == "" {
		return errors.New("no downloaded model fits this machine; download one first, or name one with --model")
	}
	plan, err := svc.PlanBenchmark(assessment, model)
	if err != nil {
		return err
	}
	opts := a.reportOptions(nil)
	if err := a.writeText(report.RenderPlan(plan, opts)); err != nil {
		return err
	}
	if !yes && !a.ask("Measure it now?", "lo0 benchmark --model "+model+" --yes") {
		_, err := fmt.Fprintln(a.Err, "Nothing was measured.")
		return err
	}
	fmt.Fprintln(a.Err)
	out, applyErr := svc.Migrate(ctx, plan, a.stepPrinter(plan, opts))
	if applyErr != nil || !out.Complete() {
		if err := a.writeText(report.RenderOutcome(plan, out, opts)); err != nil {
			return err
		}
		return applyErr
	}
	// The measurements change what lo0 estimates, so the result is read
	// back the way every other command will now see it.
	after, err := svc.Assess(ctx, nil)
	if err != nil {
		return err
	}
	for _, c := range after.Match.Candidates {
		if c.Model.ID == model {
			return a.writeText("\n" + report.ModelDetail(c, after.Match, opts) + "\n")
		}
	}
	return nil
}

// installedBest is the best-fitting model that is downloaded.
func installedBest(a app.Assessment) string {
	for _, c := range a.Match.Candidates {
		if c.Installed != nil {
			return c.Model.ID
		}
	}
	return ""
}
