package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"

	"lo0.ai/lo0/internal/bench"
	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/runtimes"
)

// Benchmarker is a backend that can measure a model in its runtime.
type Benchmarker interface {
	// Benchmark returns one step per context size, each reading a fresh
	// prompt of that size with nothing cached and writing a reply, and
	// adding what it measured to into.
	Benchmark(rt runtimes.Runtime, model string, contexts []int, into *bench.Result) []migrate.Step
}

// benchmarkContexts are the sizes every benchmark measures, before the
// user's own: short enough to be quick, long enough to show the slope.
var benchmarkContexts = []int{8 << 10, 32 << 10}

// SwapMeter reads how much the system has moved to swap so far.
type SwapMeter interface {
	SwappedOut(ctx context.Context) (uint64, error)
}

// maxSwap is how much the system may move to swap during one measurement
// before it is the machine being measured rather than the model.
const maxSwap = 256 << 20

// PlanBenchmark plans measuring an installed model on this machine at 8k
// and 32k of context, and at the reference context when that is longer, so
// its speeds are measured rather than estimated from then on. It changes
// nothing; the plan says how long each step should take.
func (s *Service) PlanBenchmark(a Assessment, model string) (migrate.Plan, error) {
	if err := onThisMachine(a); err != nil {
		return migrate.Plan{}, err
	}
	i := slices.IndexFunc(a.Match.Candidates, func(c match.Candidate) bool { return c.Model.ID == model })
	if i < 0 {
		return migrate.Plan{}, fmt.Errorf("%s does not fit this machine, or is not in the catalogue", model)
	}
	c := a.Match.Candidates[i]
	if c.Installed == nil {
		return migrate.Plan{}, fmt.Errorf("%s is not downloaded; download it first", c.Model.Name)
	}
	var rt runtimes.Runtime
	for _, r := range a.Scan.Runtimes {
		if r.ID == c.Installed.Runtime {
			rt = r
		}
	}
	var b Benchmarker
	for _, backend := range s.migrator.Backends {
		if x, ok := backend.(Benchmarker); ok && backend.ID() == rt.ID {
			b = x
		}
	}
	if b == nil {
		return migrate.Plan{}, fmt.Errorf("lo0 cannot measure models in %s", rt.ID.Name())
	}
	if s.migrator.Benchmarks == "" {
		return migrate.Plan{}, errors.New("app: this service has nowhere to keep measurements")
	}

	o := c.Installed.Option
	contexts := slices.Clone(benchmarkContexts)
	if ref := min(a.Match.ReferenceContext, o.MaxContext-4096); ref > contexts[len(contexts)-1]*3/2 {
		contexts = append(contexts, ref)
	}
	into := &bench.Result{
		Model: c.Model.ID, Quant: o.Quant.Name, KV: string(o.KV), Runtime: rt.ID, RuntimeVersion: rt.Version,
		Device: match.Device(a.Scan.Machine),
	}
	steps := b.Benchmark(rt, c.Installed.Tag, contexts, into)
	for i, n := range contexts {
		if i < len(steps) {
			steps[i].Detail = append(steps[i].Detail, "About "+format.Seconds(expected(c, o, n))+" at the speed lo0 expects.")
			steps[i].Run = s.watchSwap(steps[i].Run, into)
		}
	}
	cat := s.catalog
	path := s.migrator.Benchmarks
	steps = append(steps, migrate.Step{
		Kind:   migrate.KindMeasure,
		Title:  "Keep the measurements",
		Detail: []string{path, "Every speed lo0 shows for " + c.Model.Name + " on this machine is measured from then on."},
		Run: func(_ context.Context, fx migrate.Effects, progress func(done, total int64)) error {
			if len(into.Points) == 0 {
				return errors.New("nothing was measured")
			}
			if err := match.CheckMeasurement(cat, *into); err != nil {
				return fmt.Errorf("%w; nothing was kept, so measure again with fewer apps open", err)
			}
			into.At = fx.Now()
			old, err := fx.Files.ReadFile(path)
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("reading %s: %w", path, err)
			}
			results, err := bench.Decode(old)
			if err != nil {
				return err
			}
			data, err := bench.Encode(bench.Merge(results, *into))
			if err != nil {
				return err
			}
			if err := fx.Files.WriteFile(path, data, 0o600); err != nil {
				return fmt.Errorf("writing %s: %w", path, err)
			}
			progress(1, 1)
			return nil
		},
	})
	return migrate.Plan{
		Command: "benchmark", Runtime: rt.ID, RuntimeName: rt.Name, Model: c.Model.Name, Quant: o.Quant.Name,
		Steps: steps, Measured: into,
	}, nil
}

// watchSwap fails a measurement during which the system swapped, and drops
// the point it took: under memory pressure the speed is the machine's, not
// the model's. Where lo0 cannot read swap activity, it measures without
// the check.
func (s *Service) watchSwap(run migrate.Action, into *bench.Result) migrate.Action {
	meter := s.migrator.Swap
	if meter == nil {
		return run
	}
	return func(ctx context.Context, fx migrate.Effects, progress func(done, total int64)) error {
		before, err := meter.SwappedOut(ctx)
		if err != nil {
			return run(ctx, fx, progress)
		}
		if err := run(ctx, fx, progress); err != nil {
			return err
		}
		after, err := meter.SwappedOut(ctx)
		if err != nil || after < before || after-before <= maxSwap {
			return err
		}
		if n := len(into.Points); n > 0 {
			into.Points = into.Points[:n-1]
		}
		return fmt.Errorf("the system moved %s to swap during the measurement, so its speed is the machine's under memory pressure, not the model's; close other apps and measure again",
			format.Bytes(after-before))
	}
}

// expected is how long reading a fresh context of tokens and writing a
// reply should take with c's current curve.
func expected(c match.Candidate, o match.Option, tokens int) float64 {
	cache := float64(c.Model.Attention.KVBytes(tokens, o.KV) - c.Model.Attention.KVBytes(0, o.KV))
	read, write := o.Curve.ReadCold(cache), o.Curve.WriteAt(cache)
	if read <= 0 || write <= 0 {
		return 0
	}
	return float64(tokens)/read + float64(BenchmarkReply)/write
}

// BenchmarkReply is how many tokens each benchmark step writes.
const BenchmarkReply = 300

// benchmarks reads lo0's stored measurements. A missing file has none.
func (s *Service) benchmarks() ([]bench.Result, error) {
	if s.migrator.Benchmarks == "" {
		return nil, nil
	}
	data, err := s.files().ReadFile(s.migrator.Benchmarks)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", s.migrator.Benchmarks, err)
	}
	return bench.Decode(data)
}
