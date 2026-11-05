package tui

import (
	tea "charm.land/bubbletea/v2"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/ui/report"
	"lo0.ai/lo0/internal/ui/theme"
)

type screenID int

// Screens in sidebar order.
const (
	screenHome screenID = iota
	screenModels
	screenHarnesses
	screenInstalled
	screenClaude
)

var screenOrder = []screenID{screenHome, screenModels, screenHarnesses, screenInstalled, screenClaude}

var screenNames = [...]string{
	screenHome:      "Home",
	screenModels:    "Models",
	screenHarnesses: "Harnesses",
	screenInstalled: "Installed",
	screenClaude:    "Your Claude",
}

// screenQuestions title each screen with the question it answers.
var screenQuestions = [...]string{
	screenHome:      "Your local AI setup",
	screenModels:    "What can you run, and how well?",
	screenHarnesses: "Where do you work with a model?",
	screenInstalled: "What is on this machine?",
	screenClaude:    "How much of your Claude Code work can run locally?",
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
