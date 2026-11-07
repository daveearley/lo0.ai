package report

import (
	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/format"
)

// Screen names, as every front end labels its pages.
const (
	ScreenHome     = "Home"
	ScreenSessions = "Your sessions"
	ScreenModels   = "Models"
	ScreenAgents   = "Agents"
	ScreenMachine  = "This machine"
)

// Labels and phrases Home uses in every front end.
const (
	LabelReply   = "How long a reply takes"
	LabelRoutes  = "Where your commands go"
	LabelNext    = "What to do next"
	SharePhrase  = "of your sessions can run on this machine today"
	NoLocalModel = "no local model set up yet"
)

// HomeTitle says what Home is about: local models for the harnesses whose
// history lo0 read, or for the machine alone when it found none.
func HomeTitle(a app.Assessment) string {
	if names := Harnesses(a.Cost); names != "" && a.Scan.Usage.Sessions > 0 {
		return "Local models for your " + names + " work"
	}
	return "Local models for this machine"
}

// FitLine says how many of the user's sessions fit in a model's window, such
// as "31 of 50 sessions fit in Coder-30B-A3B’s 256k window".
func FitLine(fit, total int, model string, window int) string {
	verb := " sessions fit in "
	if fit == 1 {
		verb = " sessions fits in "
	}
	if total == 1 {
		verb = " session fits in "
	}
	return format.Int(fit) + " of " + format.Int(total) + verb + model + "’s " + format.Tokens(window) + " window"
}
