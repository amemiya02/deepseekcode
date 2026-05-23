package tui

import "github.com/charmbracelet/bubbles/key"

// keymap centralizes keybindings so the help overlay can list them.
type keymap struct {
	Submit   key.Binding
	Quit     key.Binding
	Help     key.Binding
	Cancel   key.Binding
	ScrollUp key.Binding
	ScrollDn key.Binding
	PageUp   key.Binding
	PageDn   key.Binding
	Top      key.Binding
	Bottom   key.Binding
	ToggleR  key.Binding
	ToggleRR key.Binding
	EnterN   key.Binding // switch to Normal (scroll) mode
	EnterI   key.Binding // switch to Insert (typing) mode
	Expand   key.Binding // expand last collapsed tool result
	Pager    key.Binding // open pager for last tool result
}

func defaultKeymap() keymap {
	return keymap{
		Submit:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "send")),
		Quit:     key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("^D", "quit")),
		Help:     key.NewBinding(key.WithKeys("ctrl+/"), key.WithHelp("^/", "help")),
		Cancel:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel run")),
		ScrollUp: key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("k/↑", "scroll up")),
		ScrollDn: key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("j/↓", "scroll down")),
		PageUp:   key.NewBinding(key.WithKeys("ctrl+u", "pgup"), key.WithHelp("^U", "page up")),
		PageDn:   key.NewBinding(key.WithKeys("ctrl+d", "pgdown"), key.WithHelp("^D", "page down")),
		Top:      key.NewBinding(key.WithKeys("g"), key.WithHelp("gg", "top")),
		Bottom:   key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),
		ToggleR:  key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "toggle last thinking")),
		ToggleRR: key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "toggle all thinking")),
		EnterN:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "scroll mode")),
		EnterI:   key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "type mode")),
		Expand:   key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "expand result")),
		Pager:    key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "pager")),
	}
}
