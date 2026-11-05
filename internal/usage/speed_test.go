package usage_test

import (
	"testing"
	"time"

	"lo0.ai/lo0/internal/usage"
)

func TestSpeedsMeasureEachHarnessAndModel(t *testing.T) {
	newest := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	reply := func(model string, ago time.Duration, output int, seconds float64) usage.Reply {
		return usage.Reply{Model: model, At: newest.Add(-ago), Output: output, Duration: time.Duration(seconds * float64(time.Second))}
	}
	claude := usage.Session{Harness: "Claude Code", Turns: 5, Replies: []usage.Reply{
		reply("claude-opus-5-5", 0, 600, 6),
		reply("claude-opus-5-5", time.Hour, 500, 5),
		reply("claude-opus-5-5", 2*time.Hour, 900, 10),
		// Too short to measure writing, logged with its input, left waiting.
		reply("claude-opus-5-5", 3*time.Hour, 150, 1),
		reply("claude-opus-5-5", 4*time.Hour, 400, 0.2),
		reply("claude-opus-5-5", 5*time.Hour, 400, 900),
		reply("claude-fable-5", 60*24*time.Hour, 400, 8),
	}}
	codex := usage.Session{Harness: "Codex", Turns: 1, Replies: []usage.Reply{
		// Codex's newest reply is its own reference point for "recent".
		reply("gpt-5.5", 90*24*time.Hour, 450, 10),
	}}
	p := usage.Aggregate([]usage.Session{claude, codex}, 0, identityRepos{})
	if len(p.Speeds) != 3 {
		t.Fatalf("speeds = %+v, want opus, gpt-5.5 and fable", p.Speeds)
	}
	opus, gpt, fable := p.Speeds[0], p.Speeds[1], p.Speeds[2]
	if opus.Model != "claude-opus-5-5" || opus.Replies != 3 || opus.Recent != 3 || opus.TokensPerSecond != 100 ||
		opus.ReplyTokens != 600 || opus.ReplySeconds != 6 {
		t.Errorf("opus = %+v, want 3 replies at a median 100 tok/s, 600 tokens in 6s", opus)
	}
	if gpt.Harness != "Codex" || gpt.Recent != 1 || gpt.TokensPerSecond != 45 {
		t.Errorf("gpt = %+v, want recent within Codex's own history", gpt)
	}
	if fable.Recent != 0 || fable.Replies != 1 {
		t.Errorf("fable = %+v, want one reply, none recent", fable)
	}
}
