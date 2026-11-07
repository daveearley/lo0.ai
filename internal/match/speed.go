package match

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"lo0.ai/lo0/internal/bench"
	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/machine"
)

// Source says where a speed comes from, best evidence first.
type Source string

// Speed sources.
const (
	// SourceMeasured is this model, build and runtime, measured on this
	// machine.
	SourceMeasured Source = "measured"
	// SourceMachine is an estimate calibrated with other models measured on
	// this machine.
	SourceMachine Source = "machine"
	// SourcePublished is an estimate from published results for this chip
	// or card.
	SourcePublished Source = "published"
	// SourceBandwidth is an estimate from the chip's memory bandwidth alone.
	SourceBandwidth Source = "bandwidth"
)

// Curve is a model's speed as a cost per token that grows with the context
// cache: writing a token takes Write + WriteSlope × cache bytes seconds,
// and reading one at that point Read + ReadSlope × cache bytes.
type Curve struct {
	Write      float64 `json:"write"`
	WriteSlope float64 `json:"writeSlope"`
	Read       float64 `json:"read"`
	ReadSlope  float64 `json:"readSlope"`
	Source     Source  `json:"source"`
	// MeasuredAt is when a measured curve was taken.
	MeasuredAt time.Time `json:"measuredAt,omitzero"`
}

// WriteAt is the writing speed with cache bytes of context, in tokens a
// second.
func (c Curve) WriteAt(cache float64) float64 { return perSecond(c.Write + c.WriteSlope*cache) }

// ReadAt is the speed of reading new tokens into a context already holding
// cache bytes, as a session does with each tool result.
func (c Curve) ReadAt(cache float64) float64 { return perSecond(c.Read + c.ReadSlope*cache) }

// ReadCold is the average speed of reading a whole context, from empty to
// cache bytes, with nothing cached.
func (c Curve) ReadCold(cache float64) float64 { return perSecond(c.Read + c.ReadSlope*cache/2) }

func perSecond(seconds float64) float64 {
	if seconds <= 0 {
		return 0
	}
	return 1 / seconds
}

// How a model's design changes what a machine achieves, from lo0's own
// measurements of Qwen3.6-35B-A3B and Qwen3-Coder-30B-A3B on an M5 Pro with
// Ollama 0.34.4, set out in docs/design/speed.md.
const (
	// moeWrite and moeRead are the share of a dense model's efficiency a
	// mixture-of-experts model reaches, writing and reading; its experts
	// are scattered through memory.
	moeWrite = 0.8
	moeRead  = 0.7
	// kvShare is the share of the weights' efficiency reached reading the
	// context cache while writing: attention reads it in smaller pieces.
	kvShare = 0.49
	// readCostScale sets the cost of reading a token per cache byte it
	// attends to, inversely to the machine's reading rate: about 2.3e-12
	// seconds a byte on an M5 Pro, for both models.
	readCostScale = 25.0
)

// Without published results, lo0 assumes a share of the bandwidth below
// what every listed chip reaches writing, and reads at ten parameter-tokens
// a second per byte a second of bandwidth, below every listed chip too.
const (
	bandwidthOnlyWrite = 0.6
	bandwidthOnlyRead  = 10.0
)

// rates are a machine's coefficients, from which any model's curve is
// predicted. basis says which published result they come from, if any.
type rates struct {
	bandwidth float64
	writeEff  float64
	readRate  float64
	source    Source
	basis     string
}

// machineRates are the machine's coefficients from published results for
// its chip, or from its bandwidth when it has none.
func machineRates(p machine.Profile, bw Bandwidth) rates {
	b := bw.GBps * 1e9
	if r, url, ok := publishedFor(p); ok && b > 0 {
		basis := "llama.cpp's published result for the " + r.match
		if r.gpuCores > 0 {
			basis += " with " + strconv.Itoa(r.gpuCores) + " GPU cores"
		}
		return rates{
			bandwidth: b, writeEff: r.write * referenceWeightBytes / b, readRate: r.read * referenceParams,
			source: SourcePublished, basis: basis + ", copied " + PublishedAsOf + ", " + url,
		}
	}
	return rates{bandwidth: b, writeEff: bandwidthOnlyWrite, readRate: bandwidthOnlyRead * b, source: SourceBandwidth}
}

// predict is m's curve from the machine's rates alone.
func (r rates) predict(m catalog.Model, q catalog.Quant) Curve {
	if r.bandwidth <= 0 || r.readRate <= 0 {
		return Curve{Source: r.source}
	}
	write, read := r.writeEff, r.readRate
	if m.ActiveParams > 0 && m.ActiveParams < m.Params {
		write, read = write*moeWrite, read*moeRead
	}
	weights := float64(m.ActiveParams) * q.BitsPerWeight / 8
	return Curve{
		Write:      weights / (write * r.bandwidth),
		WriteSlope: 1 / (r.writeEff * kvShare * r.bandwidth),
		Read:       float64(m.ActiveParams) / read,
		ReadSlope:  readCostScale / r.readRate,
		Source:     r.source,
	}
}

// cacheBytes is the part of m's context cache that grows with the context:
// what a token attends to.
func cacheBytes(m catalog.Model, kv catalog.KVType, tokens int) float64 {
	return float64(m.Attention.KVBytes(tokens, kv) - m.Attention.KVBytes(0, kv))
}

// fromPoints fits a curve to measurements. Two or more context sizes fit
// both cost and slope; one keeps prior's slopes.
func fromPoints(m catalog.Model, kv catalog.KVType, points []bench.Point, prior Curve) Curve {
	var write, read []xy
	for _, p := range points {
		x := cacheBytes(m, kv, p.Context)
		if p.WriteTokensPerSecond > 0 {
			write = append(write, xy{x, 1 / p.WriteTokensPerSecond})
		}
		if p.ReadTokensPerSecond > 0 {
			// A cold read's average cost is halfway up the slope.
			read = append(read, xy{x / 2, 1 / p.ReadTokensPerSecond})
		}
	}
	c := prior
	c.Write, c.WriteSlope = line(write, prior.Write, prior.WriteSlope)
	c.Read, c.ReadSlope = line(read, prior.Read, prior.ReadSlope)
	c.Source = SourceMeasured
	return c
}

type xy struct{ x, y float64 }

// line fits y = a + b·x by least squares, with b at least zero. It keeps
// slope when the points cannot set one, and cost when there are none.
func line(points []xy, cost, slope float64) (float64, float64) {
	n := float64(len(points))
	if n == 0 {
		return cost, slope
	}
	var sx, sy, sxx, sxy float64
	for _, p := range points {
		sx, sy, sxx, sxy = sx+p.x, sy+p.y, sxx+p.x*p.x, sxy+p.x*p.y
	}
	if d := n*sxx - sx*sx; len(points) > 1 && d > 0 {
		// Speeds that do not fall with context, within noise, are taken
		// at their word: no slowdown.
		b := max((n*sxy-sx*sy)/d, 0)
		return (sy - b*sx) / n, b
	}
	return max((sy-slope*sx)/n, 0), slope
}

// speeds gives each model its curve from the best evidence on this
// machine.
type speeds struct {
	device   string
	rates    rates
	measured []bench.Result
	// calibration multiplies predicted coefficients when other models were
	// measured here; calibrated is false when none were.
	calibration [4]float64
	calibrated  bool
}

// newSpeeds prepares the speed evidence for a machine: its rates and every
// model measured on it, from which a calibration for the others is drawn.
func newSpeeds(p machine.Profile, bw Bandwidth, measured []bench.Result, cat *catalog.Catalog) speeds {
	s := speeds{device: Device(p), rates: machineRates(p, bw), measured: bench.OnDevice(measured, Device(p))}
	var logs [4]float64
	n := 0
	for _, r := range s.measured {
		m, q, kv, ok := measuredBuild(cat, r)
		if !ok {
			continue
		}
		pred := s.rates.predict(m, q)
		meas := fromPoints(m, kv, r.Points, pred)
		ratios := [4]float64{meas.Write / pred.Write, meas.WriteSlope / pred.WriteSlope, meas.Read / pred.Read, meas.ReadSlope / pred.ReadSlope}
		usable := true
		for _, v := range ratios {
			usable = usable && v > 0 && !math.IsInf(v, 0) && !math.IsNaN(v)
		}
		if !usable {
			continue
		}
		for i, v := range ratios {
			logs[i] += math.Log(v)
		}
		n++
	}
	if n > 0 {
		for i := range logs {
			s.calibration[i] = math.Exp(logs[i] / float64(n))
		}
		s.calibrated = true
	}
	return s
}

// curve is m's speed at quant q, cache kv and offload off, from the best
// evidence. Measurements were taken on the GPU alone, so an offloaded curve
// takes only their calibration.
func (s speeds) curve(m catalog.Model, q catalog.Quant, kv catalog.KVType, off Offload) Curve {
	pred := s.rates.predict(m, q)
	if off.Layers > 0 {
		return offload(s.calibrate(pred), m, q, off)
	}
	for _, r := range s.measured {
		if r.Model == m.ID && r.Quant == q.Name && r.KV == string(kv) {
			c := fromPoints(m, kv, r.Points, s.calibrate(pred))
			c.MeasuredAt = r.At
			return c
		}
	}
	return s.calibrate(pred)
}

func (s speeds) calibrate(c Curve) Curve {
	if !s.calibrated {
		return c
	}
	k := s.calibration
	return Curve{Write: c.Write * k[0], WriteSlope: c.WriteSlope * k[1], Read: c.Read * k[2], ReadSlope: c.ReadSlope * k[3], Source: SourceMachine}
}

// measuredBuild finds a measured result's model and build in the
// catalogue.
func measuredBuild(cat *catalog.Catalog, r bench.Result) (catalog.Model, catalog.Quant, catalog.KVType, bool) {
	if cat == nil {
		return catalog.Model{}, catalog.Quant{}, "", false
	}
	for _, m := range cat.Models {
		if m.ID != r.Model {
			continue
		}
		for _, q := range m.Quants {
			if q.Name == r.Quant {
				return m, q, catalog.KVType(r.KV), true
			}
		}
	}
	return catalog.Model{}, catalog.Quant{}, "", false
}

// Device names the machine's GPU as measurements record it, so a result is
// only ever applied to the machine it was taken on.
func Device(p machine.Profile) string {
	if len(p.GPUs) == 0 {
		return p.CPU.Model
	}
	g := p.GPUs[0]
	if g.Cores > 0 {
		return g.Name + " " + strconv.Itoa(g.Cores) + "-core"
	}
	return g.Name
}

// maxResidual is how far a measurement may sit from the curve fitted
// through all of them. Clean runs on lo0's test machine sat within 12%; a
// run disturbed by swapping sat 50% off.
const maxResidual = 0.25

// CheckMeasurement reports whether a model's measurements follow one
// curve, as an undisturbed run's do. One that sits far off the others
// means the machine was busy while it was taken.
func CheckMeasurement(cat *catalog.Catalog, r bench.Result) error {
	m, _, kv, ok := measuredBuild(cat, r)
	if !ok || len(r.Points) < 3 {
		return nil
	}
	c := fromPoints(m, kv, r.Points, Curve{})
	for _, p := range r.Points {
		at := cacheBytes(m, kv, p.Context)
		for _, pair := range []struct {
			what          string
			got, measured float64
		}{{"writing", c.WriteAt(at), p.WriteTokensPerSecond}, {"reading", c.ReadCold(at), p.ReadTokensPerSecond}} {
			if pair.measured > 0 && math.Abs(pair.got-pair.measured)/pair.measured > maxResidual {
				return fmt.Errorf("the measurements do not follow one curve: %s at %s measured %.0f tok/s where the others imply %.0f, which happens when the machine is busy",
					pair.what, strconv.Itoa(p.Context)+" tokens", pair.measured, pair.got)
			}
		}
	}
	return nil
}
