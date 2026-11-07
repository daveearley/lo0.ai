package tui

import (
	tea "charm.land/bubbletea/v2"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/ui/report"
	"lo0.ai/lo0/internal/ui/theme"
)

type screenID int

// Screens, in the order the menu lists them.
const (
	screenHome screenID = iota
	screenSessions
	screenModels
	screenAgents
	screenMachine
)

var screenOrder = []screenID{screenHome, screenSessions, screenModels, screenAgents, screenMachine}

var screenNames = [...]string{
	screenHome:     report.ScreenHome,
	screenSessions: report.ScreenSessions,
	screenModels:   report.ScreenModels,
	screenAgents:   report.ScreenAgents,
	screenMachine:  report.ScreenMachine,
}

// screenQuestions title each screen with the question it answers.
var screenQuestions = [...]string{
	screenHome:     "Local models for this machine",
	screenSessions: "How much of your work can run locally?",
	screenModels:   "What can you run, and how well?",
	screenAgents:   "What carries over to each coding agent?",
	screenMachine:  "What is on this machine?",
}

// screen is one page of the interface. Screens are values: update returns
// the next state rather than mutating.
type screen interface {
	update(msg tea.Msg) (screen, tea.Cmd)
	view(width, height int) string
	// hints are the keys worth showing in the footer right now.
	hints() []hint
	// capturesKeys is true while the screen needs every key, for example
	// while a filter is being typed.
	capturesKeys() bool
}

// titled is implemented by screens whose question depends on their state.
type titled interface {
	title() string
}

// helper is implemented by screens with more keys than the footer can show.
type helper interface {
	help() []hint
}

// nester is implemented by screens with views inside them, such as an open
// detail or a filter, that esc leaves before it leaves the screen.
type nester interface {
	nested() bool
}

// clickMsg is a mouse click at a position in the screen's content area.
type clickMsg struct{ x, y int }

// env is what every screen renders from.
type env struct {
	theme      theme.Theme
	keys       keyMap
	assessment app.Assessment
	home       string
	// export is true when the report can be saved, try when a model can
	// be tried and change when lo0 can change what is installed.
	export, try, change bool
	// inventory is what is installed, and inventoryErr why part of it
	// could not be read.
	inventory    app.Inventory
	inventoryErr error
	// harnesses describes each harness as it is on this machine.
	harnesses []app.HarnessView
}

func (e env) reportOptions(width int) report.Options {
	return report.Options{Width: width, Theme: e.theme, Home: e.home}
}

// navigateMsg asks the root to switch screens.
type navigateMsg struct {
	to screenID
	// then is sent to the destination screen after switching.
	then tea.Msg
}

func navigate(to screenID, then tea.Msg) tea.Cmd {
	return func() tea.Msg { return navigateMsg{to: to, then: then} }
}
