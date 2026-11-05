package version_test

import (
	"testing"

	"lo0.ai/lo0/internal/version"
)

func TestCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"2.1.10", "2.1.9", 1},
		{"2.1.317", "2.1.317", 0},
		{"2.1.221 (Claude Code)", "2.1.221", 0},
		{"2.0", "2.0.1", -1},
		{"3.0.0", "2.99.99", 1},
		{"", "0.0.1", -1},
		{"", "", 0},
		{"2.1.5-beta", "2.1.5", 0},
		{"garbage", "1.0", -1},
	}
	for _, tt := range tests {
		if got := version.Compare(tt.a, tt.b); got != tt.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestParse(t *testing.T) {
	tests := map[string]string{
		"2.1.221 (Claude Code)\n": "2.1.221",
		"v2.1.300":                "2.1.300",
		"2.1.309":                 "2.1.309",
		"2.1.5-beta.1":            "2.1.5",
		"Claude Code":             "",
		"":                        "",
	}
	for in, want := range tests {
		if got := version.Parse(in); got != want {
			t.Errorf("Parse(%q) = %q, want %q", in, got, want)
		}
	}
}
