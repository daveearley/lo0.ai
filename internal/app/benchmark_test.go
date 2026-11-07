package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lo0.ai/lo0/internal/bench"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/migrate/migratetest"
)

func TestBenchmarkMeasuresAndIsUsed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Prompt string `json:"prompt"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		tokens := strings.Count(body.Prompt, "\n") * 47
		_ = json.NewEncoder(w).Encode(map[string]int64{
			"prompt_eval_count": int64(tokens), "prompt_eval_duration": int64(tokens) * 1_000_000,
			"eval_count": 300, "eval_duration": 6_000_000_000,
		})
	}))
	t.Cleanup(srv.Close)
	const store = "/state/benchmarks.json"
	files := migratetest.NewFiles(nil)
	mig := migrator(files)
	mig.Benchmarks = store
	mig.Effects.Client = srv.Client()

	_, a := withBestInstalled(t)
	res := a.Scan
	res.Runtimes[0].Endpoint = srv.URL
	svc, a := assess(t, res, mig)
	best, _ := a.Match.Best()
	if best.Best.Curve.Source == match.SourceMeasured {
		t.Fatal("measured before measuring")
	}
	p, err := svc.PlanBenchmark(a, best.Model.ID)
	if err != nil {
		t.Fatal(err)
	}
	if p.Command != "benchmark" || p.Changes() || len(p.Steps) < 3 || !strings.Contains(strings.Join(p.Steps[0].Detail, " "), "About ") {
		t.Fatalf("plan = %+v", p)
	}
	if out, err := svc.Migrate(t.Context(), p, nil); err != nil || !out.Complete() {
		t.Fatalf("outcome %+v, err %v", out, err)
	}
	data, ok := files.Get(store)
	if !ok {
		t.Fatal("nothing was kept")
	}
	results, err := bench.Decode([]byte(data))
	if err != nil || len(results) != 1 || len(results[0].Points) != len(p.Steps)-1 || results[0].Device != match.Device(a.Scan.Machine) {
		t.Fatalf("kept %+v, err %v", results, err)
	}

	// Assessing again uses the measurement.
	_, again := assess(t, res, mig)
	for _, c := range again.Match.Candidates {
		if c.Model.ID == best.Model.ID {
			if c.Best.Curve.Source != match.SourceMeasured || c.Best.TokensPerSecond < 49 || c.Best.TokensPerSecond > 51 {
				t.Errorf("after measuring: %v tok/s from %s, want the measured 50; curve %+v quant %s kv %s points %+v", c.Best.TokensPerSecond, c.Best.Curve.Source, c.Best.Curve, c.Best.Quant.Name, c.Best.KV, results[0].Points)
			}
		}
	}
}

func TestBenchmarkNeedsADownload(t *testing.T) {
	svc, a := installedService(t)
	best, _ := a.Match.Best()
	if _, err := svc.PlanBenchmark(a, best.Model.ID); err == nil || !strings.Contains(err.Error(), "not downloaded") {
		t.Errorf("err = %v", err)
	}
}

// swapping is a system that moves a gigabyte to swap every time it is read.
type swapping struct{ out *uint64 }

func (s swapping) SwappedOut(context.Context) (uint64, error) {
	*s.out += 1 << 30
	return *s.out, nil
}

func TestBenchmarkRefusesAMeasurementTakenWhileSwapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Prompt string `json:"prompt"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		tokens := strings.Count(body.Prompt, "\n") * 47
		_ = json.NewEncoder(w).Encode(map[string]int64{"prompt_eval_count": int64(tokens), "prompt_eval_duration": int64(tokens) * 1_000_000, "eval_count": 300, "eval_duration": 6_000_000_000})
	}))
	t.Cleanup(srv.Close)
	files := migratetest.NewFiles(nil)
	mig := migrator(files)
	mig.Benchmarks = "/state/benchmarks.json"
	mig.Effects.Client = srv.Client()
	var swapped uint64
	mig.Swap = swapping{&swapped}
	_, a := withBestInstalled(t)
	res := a.Scan
	res.Runtimes[0].Endpoint = srv.URL
	svc, a := assess(t, res, mig)
	best, _ := a.Match.Best()
	p, err := svc.PlanBenchmark(a, best.Model.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Migrate(t.Context(), p, nil)
	if err == nil || !strings.Contains(err.Error(), "to swap during the measurement") {
		t.Errorf("err = %v, want the measurement refused", err)
	}
	if _, ok := files.Get("/state/benchmarks.json"); ok {
		t.Error("a measurement taken while swapping was kept")
	}
}
