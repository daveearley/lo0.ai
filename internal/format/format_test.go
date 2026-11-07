package format_test

import (
	"testing"
	"time"

	"lo0.ai/lo0/internal/format"
)

func TestInt(t *testing.T) {
	tests := map[int]string{0: "0", 7: "7", 999: "999", 1000: "1,000", 35081: "35,081", 1234567: "1,234,567", -4200: "-4,200"}
	for in, want := range tests {
		if got := format.Int(in); got != want {
			t.Errorf("Int(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestBytes(t *testing.T) {
	tests := map[uint64]string{
		512:         "512 B",
		2048:        "2 KiB",
		512 << 20:   "512 MiB",
		64 << 30:    "64 GiB",
		18582531596: "17.3 GiB",
		2497282444:  "2.3 GiB",
		1 << 40:     "1 TiB",
		1023 << 30:  "1023 GiB",
		1048575:     "1 MiB",
	}
	for in, want := range tests {
		if got := format.Bytes(in); got != want {
			t.Errorf("Bytes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestTokens(t *testing.T) {
	tests := map[int]string{
		800: "800", 32768: "32k", 194641: "190k", 5000: "4.9k", 262144: "256k", 1 << 20: "1M", 1572864: "1.5M", 1048575: "1M",
	}
	for in, want := range tests {
		if got := format.Tokens(in); got != want {
			t.Errorf("Tokens(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestPercent(t *testing.T) {
	tests := map[float64]string{0: "0%", 0.002: "<1%", 0.0117: "1%", 0.6136: "61%", 0.998: ">99%", 1: "100%"}
	for in, want := range tests {
		if got := format.Percent(in); got != want {
			t.Errorf("Percent(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestDateRange(t *testing.T) {
	at := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 12, 0, 0, 0, time.Local) }
	tests := []struct {
		from, to time.Time
		want     string
	}{
		{at(2026, 3, 8), at(2026, 11, 4), "8 Mar – 4 Nov 2026"},
		{at(2025, 12, 30), at(2026, 1, 2), "30 Dec 2025 – 2 Jan 2026"},
		{at(2026, 11, 4), at(2026, 11, 4), "4 Nov 2026"},
		{time.Time{}, at(2026, 11, 4), ""},
	}
	for _, tt := range tests {
		if got := format.DateRange(tt.from, tt.to); got != tt.want {
			t.Errorf("DateRange = %q, want %q", got, tt.want)
		}
	}
}

func TestDuration(t *testing.T) {
	tests := map[time.Duration]string{
		640 * time.Millisecond:  "640ms",
		3200 * time.Millisecond: "3.2s",
		2 * time.Second:         "2s",
		64 * time.Second:        "1m 4s",
		150 * time.Minute:       "2h 30m",
	}
	for in, want := range tests {
		if got := format.Duration(in); got != want {
			t.Errorf("Duration(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestPlural(t *testing.T) {
	if got := format.Plural(1, "file", "files"); got != "1 file" {
		t.Errorf("Plural(1) = %q", got)
	}
	if got := format.Plural(1200, "file", "files"); got != "1,200 files" {
		t.Errorf("Plural(1200) = %q", got)
	}
}

func TestUSD(t *testing.T) {
	tests := map[float64]string{0: "$0.00", 0.4321: "$0.43", 9.999: "$10.00", 10: "$10", 14426.4: "$14,426", -3: "-$3.00"}
	for in, want := range tests {
		if got := format.USD(in); got != want {
			t.Errorf("USD(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestWords(t *testing.T) {
	tests := map[int64]string{999: "999", 84_000_000: "84 million", 24_900_000_000: "24.9 billion", 2_450_000_000: "2.5 billion", 1_200_000_000_000: "1.2 trillion"}
	for in, want := range tests {
		if got := format.Words(in); got != want {
			t.Errorf("Words(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestMultiple(t *testing.T) {
	tests := map[float64]string{0.54: "0.5×", 2: "2×", 28.9: "29×", 51.49: "51×"}
	for in, want := range tests {
		if got := format.Multiple(in); got != want {
			t.Errorf("Multiple(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestModel(t *testing.T) {
	tests := map[string]string{
		"claude-opus-5-5":           "Opus 5.5",
		"claude-fable-5":            "Fable 5",
		"claude-haiku-4-5-20251001": "Haiku 4.5",
		"claude-sonnet-5-5[1m]":     "Sonnet 5.5",
		"gpt-5.5":                   "GPT-5.5",
		"gpt-5.2-codex":             "GPT-5.2-Codex",
		"mystery":                   "mystery",
		"gpt-":                      "gpt-",
		"lo0/qwen3.6-35b-a3b:256k":  "lo0/qwen3.6-35b-a3b:256k",
		"opus-5":                    "opus-5",
	}
	for in, want := range tests {
		if got := format.Model(in); got != want {
			t.Errorf("Model(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSeconds(t *testing.T) {
	tests := map[float64]string{0.2: "1s", 6.1: "6s", 17.4: "17s", 76.2: "1m 16s"}
	for in, want := range tests {
		if got := format.Seconds(in); got != want {
			t.Errorf("Seconds(%v) = %q, want %q", in, got, want)
		}
	}
}
