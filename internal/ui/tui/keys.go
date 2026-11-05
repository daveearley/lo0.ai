package tui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	NextScreen, PrevScreen key.Binding
	Up, Down               key.Binding
	PageUp, PageDown       key.Binding
	Top, Bottom            key.Binding
	ScrollUp, ScrollDown   key.Binding
	Filter, Back, Open     key.Binding
	Palette, Help, Quit    key.Binding
	Rescan, Export         key.Binding
	Target                 key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		NextScreen: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next screen")),
		PrevScreen: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "previous screen")),
		Up:         key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑ k", "up")),
		Down:       key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓ j", "down")),
		PageUp:     key.NewBinding(key.WithKeys("pgup", "ctrl+u"), key.WithHelp("pgup", "page up")),
		PageDown:   key.NewBinding(key.WithKeys("pgdown", "ctrl+d"), key.WithHelp("pgdn", "page down")),
		Top:        key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("g", "top")),
		Bottom:     key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("G", "bottom")),
		ScrollUp:   key.NewBinding(key.WithKeys("K", "shift+up"), key.WithHelp("K", "scroll details up")),
		ScrollDown: key.NewBinding(key.WithKeys("J", "shift+down"), key.WithHelp("J", "scroll details down")),
		Filter:     key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		Back:       key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Open:       key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Palette:    key.NewBinding(key.WithKeys("ctrl+k", ":"), key.WithHelp("ctrl+k", "commands")),
		Help:       key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "keys")),
		Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Rescan:     key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "scan again")),
		Export:     key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "save the report to share")),
		Target:     key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "next harness")),
	}
}

// hint is a key shown in the footer or the help overlay.
type hint struct{ key, desc string }

func fromBinding(b key.Binding) hint {
	h := b.Help()
	return hint{h.Key, h.Desc}
}
