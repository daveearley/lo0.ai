package app_test

import (
	"errors"
	"testing"
	"time"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/bench"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/migrate/migratetest"
	"lo0.ai/lo0/internal/runtimes"
)

func TestSimulatedHardwareChangesNothing(t *testing.T) {
	_, real := withBestInstalled(t)
	best, _ := real.Match.Best()
	data, err := bench.Encode([]bench.Result{{
		Model: best.Model.ID, Quant: best.Best.Quant.Name, KV: string(best.Best.KV), Runtime: runtimes.Ollama,
		Device: match.Device(real.Scan.Machine), At: time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC),
		Points: []bench.Point{{Context: 8192, ReadTokensPerSecond: 900, WriteTokensPerSecond: 50}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	files := migratetest.NewFiles(map[string]string{"/state/benchmarks.json": string(data)})
	mig := migrator(files)
	mig.Benchmarks = "/state/benchmarks.json"

	res := real.Scan
	res.Benchmarks = nil
	if _, measured := assess(t, res, mig); len(measured.Scan.Benchmarks) == 0 {
		t.Fatal("the real machine did not take its benchmarks")
	}
	res.Machine.Simulated = true
	svc, a := assess(t, res, mig)
	if len(a.Scan.Benchmarks) != 0 {
		t.Errorf("a simulated machine took this machine's benchmarks: %+v", a.Scan.Benchmarks)
	}
	choice := app.MigrationChoice{Runtime: runtimes.Ollama, Model: best.Model.ID}
	plans := map[string]func() (migrate.Plan, error){
		"migrate":   func() (migrate.Plan, error) { return svc.PlanMigration(a, choice) },
		"try":       func() (migrate.Plan, error) { return svc.PlanTry(a, choice) },
		"pull":      func() (migrate.Plan, error) { return svc.PlanPull(a, choice) },
		"benchmark": func() (migrate.Plan, error) { return svc.PlanBenchmark(a, best.Model.ID) },
		"install":   func() (migrate.Plan, error) { return svc.PlanInstall(a, harness.OpenCode) },
		"remove":    func() (migrate.Plan, error) { return svc.PlanRemove(a, runtimes.Ollama, best.Installed.Tag) },
	}
	for name, plan := range plans {
		if _, err := plan(); !errors.Is(err, app.ErrSimulated) {
			t.Errorf("%s: err = %v, want ErrSimulated", name, err)
		}
	}
}
