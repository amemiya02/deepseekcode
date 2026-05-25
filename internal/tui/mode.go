package tui

import tea "github.com/charmbracelet/bubbletea"

// appMode controls which sub-component receives keyboard input.
//
// Insert: textarea focused, typing flows through.
// Normal: viewport focused, j/k/scroll navigation; printable chars
//
//	auto-switch back to Insert via Update.
//
// Permission: modal — permission card eats all key input until answered.
// Pager:      modal — pager overlay eats all key input until closed.
type appMode int

const (
	modeInsert appMode = iota
	modeNormal
	modePermission
	modePager
	modeVisual   // Vim-style line-range selection in the scrollback
	modeQuestion // Modal question card with selectable options
)

// setMode is the single source of truth for mode transitions. It keeps
// focus state, the status badge, and the mode field in lockstep so the
// View() rendering can never disagree with the Update() routing.
//
// Returns the tea.Cmd needed to apply focus changes (e.g. textarea
// blink). Callers should batch the result with other commands.
func (a *App) setMode(next appMode) tea.Cmd {
	if a.mode == next {
		return nil
	}
	a.mode = next
	a.status.mode = next
	switch next {
	case modeInsert:
		return a.input.Focus()
	case modeNormal, modePermission, modePager, modeVisual, modeQuestion:
		a.input.Blur()
	}
	return nil
}

// isPrintableKey returns true for keys that represent a character the
// user would expect to appear in the text input (letters, digits,
// punctuation, space). Used in Normal mode to auto-switch to Insert.
func isPrintableKey(km tea.KeyMsg) bool {
	s := km.String()
	if len(s) != 1 {
		return false
	}
	r := s[0]
	return r >= 32 && r < 127
}
