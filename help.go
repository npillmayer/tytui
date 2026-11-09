package main

import "charm.land/bubbles/v2/key"

type keyMap struct{ Run, Focus, Scroll, Newline, Help, Quit, Breaker, History, Export key.Binding }

func (k keyMap) ShortHelp() []key.Binding { return []key.Binding{k.Run, k.Focus, k.Help, k.Quit} }
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Run, k.Focus, k.Newline}, {k.Breaker, k.History, k.Export}, {k.Scroll, k.Help, k.Quit}}
}

var keys = keyMap{
	Breaker: key.NewBinding(key.WithKeys("f2"), key.WithHelp("f2", "switch breaker")),
	History: key.NewBinding(key.WithKeys("f3"), key.WithHelp("f3", "previous saved layout")),
	Export:  key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "export SVG")),
	Run:     key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "run layout")),
	Focus:   key.NewBinding(key.WithKeys("tab", "shift+tab"), key.WithHelp("tab/shift+tab", "focus")),
	Scroll:  key.NewBinding(key.WithKeys("up", "down", "pgup", "pgdown"), key.WithHelp("↑/↓ pgup/pgdn", "scroll results")),
	Newline: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "newline / run width")),
	Help:    key.NewBinding(key.WithKeys("f1"), key.WithHelp("f1", "help")),
	Quit:    key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
}
