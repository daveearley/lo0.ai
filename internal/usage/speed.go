package usage

import (
	"cmp"
	"slices"
	"time"

	"lo0.ai/lo0/internal/stats"
)

// Speed is how fast one model wrote its replies in one harness, measured
// from the harness's own logs.
type Speed struct {
	Harness string `json:"harness"`
	Model   string `json:"model"`
	// Replies counts the replies measured, and Recent those in the 30 days
	// before the harness's newest reply.
	Replies int `json:"replies"`
	Recent  int `json:"recent"`
	// TokensPerSecond is the median reply's speed, and Low and High bound
	// the middle half of replies.
	TokensPerSecond float64 `json:"tokensPerSecond"`
	Low             float64 `json:"low"`
	High            float64 `json:"high"`
	// ReplyTokens and ReplySeconds are the median reply's length and time.
	ReplyTokens  int     `json:"replyTokens"`
	ReplySeconds float64 `json:"replySeconds"`
	// NewTokens is the median input a reply read that was not cached: what
	// a local runtime would also have to read before replying.
	NewTokens int `json:"newTokens,omitzero"`
}

// Replies only say how fast a model writes once they are long enough for
// writing to outweigh the wait before the first token. A reply logged within
// half a second of its input was written to the log in one batch with it,
// as some Codex releases do, and one over ten minutes was left waiting on
// the user, so neither is timed.
const (
	minReplyTokens = 200
	minReplyTime   = 500 * time.Millisecond
	maxReplyTime   = 10 * time.Minute
	recentWindow   = 30 * 24 * time.Hour
)

type speedKey struct{ harness, model string }

// speedsOf measures each harness and model's replies, most used recently
// first.
func speedsOf(sessions []Session) []Speed {
	replies := map[speedKey][]Reply{}
	newest := map[string]time.Time{}
	for _, s := range sessions {
		for _, r := range s.Replies {
			if r.Model == "" || r.Output < minReplyTokens || r.Duration < minReplyTime || r.Duration > maxReplyTime {
				continue
			}
			k := speedKey{s.Harness, r.Model}
			replies[k] = append(replies[k], r)
			if r.At.After(newest[s.Harness]) {
				newest[s.Harness] = r.At
			}
		}
	}
	out := make([]Speed, 0, len(replies))
	for k, rs := range replies {
		rates := make([]float64, len(rs))
		tokens := make([]int, len(rs))
		fresh := make([]int, len(rs))
		seconds := make([]float64, len(rs))
		recent := 0
		for i, r := range rs {
			seconds[i] = r.Duration.Seconds()
			rates[i] = float64(r.Output) / seconds[i]
			tokens[i] = r.Output
			fresh[i] = r.NewTokens
			if newest[k.harness].Sub(r.At) <= recentWindow {
				recent++
			}
		}
		out = append(out, Speed{
			Harness: k.harness, Model: k.model, Replies: len(rs), Recent: recent,
			TokensPerSecond: stats.Percentile(rates, 50),
			Low:             stats.Percentile(rates, 25),
			High:            stats.Percentile(rates, 75),
			ReplyTokens:     stats.Percentile(tokens, 50),
			ReplySeconds:    stats.Percentile(seconds, 50),
			NewTokens:       stats.Percentile(fresh, 50),
		})
	}
	slices.SortFunc(out, func(a, b Speed) int {
		return cmp.Or(cmp.Compare(b.Recent, a.Recent), cmp.Compare(b.Replies, a.Replies),
			cmp.Compare(a.Harness, b.Harness), cmp.Compare(a.Model, b.Model))
	})
	return out
}
