package match

import (
	"math"

	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/usage"
)

// Reference is the speed a user is used to in one harness: the model they
// used most in its last 30 days, among those with enough replies to
// measure, as measured from their own logs.
type Reference struct {
	Harness string `json:"harness"`
	// Model is the model's ID and Name its product name.
	Model           string  `json:"model"`
	Name            string  `json:"name"`
	Replies         int     `json:"replies"`
	TokensPerSecond float64 `json:"tokensPerSecond"`
	// ReplyTokens and ReplySeconds are the median reply's length and time,
	// and NewTokens the median input a reply read that was not cached.
	ReplyTokens  int     `json:"replyTokens"`
	ReplySeconds float64 `json:"replySeconds"`
	NewTokens    int     `json:"newTokens,omitzero"`
}

// Band says how a local model's speed compares with what the user is used
// to.
type Band string

// Pace bands.
const (
	BandComparable Band = "comparable"
	BandSlower     Band = "slower"
	BandVerySlow   Band = "very slow"
)

// Pace compares a model's speed with the user's main reference over a
// typical reply: reading what is new since the last one, then writing it.
type Pace struct {
	Band Band `json:"band"`
	// Slower is how many times as long a typical reply takes as with the
	// reference. ReplySeconds is that reply's time, ReadSeconds the part
	// spent reading. All are zero without a reference.
	Slower       float64 `json:"slower,omitzero"`
	ReplySeconds float64 `json:"replySeconds,omitzero"`
	ReadSeconds  float64 `json:"readSeconds,omitzero"`
}

const (
	// minReferenceReplies is how many measured replies a model needs before
	// lo0 trusts its speed as a reference.
	minReferenceReplies = 200
	// slowerShare and verySlowShare are the shares of the reference's speed
	// below which a model is slower and very slow.
	slowerShare   = 0.5
	verySlowShare = 0.2
)

// references picks one reference per harness, the harness whose reference
// was used most recently first.
func references(p usage.Profile) []Reference {
	var out []Reference
	seen := map[string]bool{}
	for _, s := range p.Speeds {
		if seen[s.Harness] || s.Replies < minReferenceReplies || s.TokensPerSecond <= 0 {
			continue
		}
		// A reference is a hosted model, one lo0 can name. A local model
		// the user has moved to is what is being compared, not the yardstick.
		if format.Model(s.Model) == s.Model {
			continue
		}
		seen[s.Harness] = true
		out = append(out, Reference{
			Harness: s.Harness, Model: s.Model, Name: format.Model(s.Model), Replies: s.Replies,
			TokensPerSecond: s.TokensPerSecond, ReplyTokens: s.ReplyTokens, ReplySeconds: s.ReplySeconds, NewTokens: s.NewTokens,
		})
	}
	return out
}

// PaceOf bands a model's writing and reading speeds, estimated or
// measured, against the main reference, the first of refs.
func PaceOf(write, read float64, refs []Reference) Pace {
	return paceOf(write, read, refs)
}

// paceOf bands a model against the main reference by the time a typical
// reply takes. Without a reference it uses fixed thresholds for agentic
// work on writing speed alone.
func paceOf(write, read float64, refs []Reference) Pace {
	if write <= 0 {
		return Pace{Band: BandVerySlow}
	}
	if len(refs) == 0 {
		switch {
		case write < minSpeed:
			return Pace{Band: BandVerySlow}
		case write < comfortableSpeed:
			return Pace{Band: BandSlower}
		default:
			return Pace{Band: BandComparable}
		}
	}
	ref := refs[0]
	p := Pace{ReplySeconds: float64(ref.ReplyTokens) / write}
	if read > 0 {
		p.ReadSeconds = float64(ref.NewTokens) / read
		p.ReplySeconds += p.ReadSeconds
	}
	if ref.ReplySeconds > 0 {
		p.Slower = p.ReplySeconds / ref.ReplySeconds
	} else {
		p.Slower = ref.TokensPerSecond / write
	}
	switch {
	case write < minSpeed || p.Slower > 1/verySlowShare:
		p.Band = BandVerySlow
	case p.Slower > 1/slowerShare:
		p.Band = BandSlower
	default:
		p.Band = BandComparable
	}
	return p
}

// AboutAsFast is how close to the reference a model must come to be called
// about as fast, rather than a multiple slower.
const AboutAsFast = 1.15

// PaceSentence says in words how p compares with ref: how long a typical
// reply takes against the reference, and how much of it is reading.
func PaceSentence(p Pace, ref Reference) string {
	where := ref.Name + " in your " + ref.Harness + " sessions"
	if p.Slower < AboutAsFast {
		return "A typical reply takes about as long as with " + where + "."
	}
	lead := "A typical reply takes "
	if p.Band == BandVerySlow {
		lead = "That is very slow: a typical reply takes "
	}
	// The parts are rounded first and the whole is their sum, so the
	// sentence adds up.
	read, write := math.Round(p.ReadSeconds), math.Round(p.ReplySeconds-p.ReadSeconds)
	out := lead + format.Seconds(read+write) + ", against " + format.Seconds(ref.ReplySeconds) + " with " + where
	if read >= 1 {
		out += ": " + format.Seconds(read) + " reading what is new and " + format.Seconds(write) + " writing"
	}
	return out + "."
}
