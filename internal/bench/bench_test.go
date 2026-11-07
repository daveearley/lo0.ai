package bench_test

import (
	"testing"
	"time"

	"lo0.ai/lo0/internal/bench"
	"lo0.ai/lo0/internal/runtimes"
)

func result(model string, at time.Time, device string) bench.Result {
	return bench.Result{
		Model: model, Quant: "Q4_K_M", KV: "f16", Runtime: runtimes.Ollama, Device: device, At: at,
		Points: []bench.Point{{Context: 8000, ReadTokensPerSecond: 1400, WriteTokensPerSecond: 70}},
	}
}

func TestMergeReplacesAnOlderMeasurement(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 11, d, 9, 0, 0, 0, time.UTC) }
	results := []bench.Result{result("a", day(1), "M5 Pro"), result("b", day(1), "M5 Pro"), result("a", day(1), "M4 Pro")}
	results = bench.Merge(results, result("a", day(5), "M5 Pro"))
	if len(results) != 3 {
		t.Fatalf("results = %d, want the old a on the M5 Pro replaced", len(results))
	}
	got, ok := bench.For(results, "M5 Pro", "a", "Q4_K_M", "f16", runtimes.Ollama)
	if !ok || !got.At.Equal(day(5)) {
		t.Errorf("For = %+v, %v", got, ok)
	}
	if on := bench.OnDevice(results, "M5 Pro"); len(on) != 2 || on[0].Model != "a" {
		t.Errorf("OnDevice = %+v, want this device's two, newest first", on)
	}
}

func TestEncodeAndDecode(t *testing.T) {
	r := []bench.Result{result("a", time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC), "M5 Pro")}
	data, err := bench.Encode(r)
	if err != nil {
		t.Fatal(err)
	}
	back, err := bench.Decode(data)
	if err != nil || len(back) != 1 || back[0].Points[0].WriteTokensPerSecond != 70 {
		t.Errorf("decoded %+v, err %v", back, err)
	}
	if none, err := bench.Decode(nil); err != nil || none != nil {
		t.Errorf("empty file = %v, %v", none, err)
	}
	if _, err := bench.Decode([]byte("{")); err == nil {
		t.Error("a broken file decoded")
	}
}
