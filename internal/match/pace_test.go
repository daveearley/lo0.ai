package match

import (
	"testing"

	"lo0.ai/lo0/internal/usage"
)

func TestReferencesPickOnePerHarness(t *testing.T) {
	p := usage.Profile{Speeds: []usage.Speed{
		{Harness: "Codex", Model: "gpt-6-astra", Replies: 1, Recent: 1, TokensPerSecond: 17},
		{Harness: "Claude Code", Model: "lo0/qwen3.6-35b-a3b:256k", Replies: 30_000, Recent: 30_000, TokensPerSecond: 30},
		{Harness: "Claude Code", Model: "claude-opus-5-5", Replies: 23_000, Recent: 19_000, TokensPerSecond: 96, ReplyTokens: 565, ReplySeconds: 6.1},
		{Harness: "Claude Code", Model: "claude-sonnet-5-5", Replies: 300, Recent: 300, TokensPerSecond: 130},
		{Harness: "Codex", Model: "gpt-5.5", Replies: 968, TokensPerSecond: 44},
	}}
	refs := references(p)
	if len(refs) != 2 || refs[0].Model != "claude-opus-5-5" || refs[0].Name != "Opus 5.5" || refs[1].Model != "gpt-5.5" {
		t.Errorf("references = %+v, want Opus 5.5 then GPT-5.5, skipping a local model and one with one reply", refs)
	}
}

func TestPaceBands(t *testing.T) {
	// A typical Opus 5.5 reply reads 1,289 new tokens and writes 565 in 6s.
	opus := []Reference{{Name: "Opus 5.5", Harness: "Claude Code", TokensPerSecond: 96, ReplyTokens: 565, ReplySeconds: 6, NewTokens: 1289}}
	for _, tt := range []struct {
		name        string
		write, read float64
		refs        []Reference
		want        Band
	}{
		{"under twice as long", 200, 2000, opus, BandComparable},
		{"two to five times as long", 40, 400, opus, BandSlower},
		{"over five times as long", 15, 200, opus, BandVerySlow},
		{"slow reading alone makes it very slow", 90, 30, opus, BandVerySlow},
		{"under 10 tok/s whatever the reference", 9, 5000, opus, BandVerySlow},
		{"no speed at all", 0, 0, opus, BandVerySlow},
		{"no reference, comfortable", 30, 0, nil, BandComparable},
		{"no reference, below comfortable", 20, 0, nil, BandSlower},
		{"no reference, under 10", 7, 0, nil, BandVerySlow},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := paceOf(tt.write, tt.read, tt.refs); got.Band != tt.want {
				t.Errorf("paceOf(%v, %v) = %+v, want %s", tt.write, tt.read, got, tt.want)
			}
		})
	}
	p := paceOf(56.5, 1289, opus)
	if p.ReplySeconds != 11 || p.ReadSeconds != 1 || p.Slower < 1.83 || p.Slower > 1.84 {
		t.Errorf("pace = %+v, want 1s reading and 10s writing, 11s against 6s", p)
	}
}

func TestPaceSentence(t *testing.T) {
	opus := Reference{Name: "Opus 5.5", Harness: "Claude Code", TokensPerSecond: 96, ReplyTokens: 565, ReplySeconds: 6.1, NewTokens: 1289}
	for _, tt := range []struct {
		write, read float64
		want        string
	}{
		{31, 230, "A typical reply takes 24s, against 6s with Opus 5.5 in your Claude Code sessions: 6s reading what is new and 18s writing."},
		{7.4, 300, "That is very slow: a typical reply takes 1m 20s, against 6s with Opus 5.5 in your Claude Code sessions: 4s reading what is new and 1m 16s writing."},
		{110, 100_000, "A typical reply takes about as long as with Opus 5.5 in your Claude Code sessions."},
	} {
		if got := PaceSentence(paceOf(tt.write, tt.read, []Reference{opus}), opus); got != tt.want {
			t.Errorf("at %v/%v tok/s:\n got %q\nwant %q", tt.write, tt.read, got, tt.want)
		}
	}
}
