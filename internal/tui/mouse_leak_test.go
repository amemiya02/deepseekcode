package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestIsLeakedMouseSeq(t *testing.T) {
	cases := []struct {
		name  string
		runes []rune
		want  bool
	}{
		{"sgr wheel-down full", []rune("[<65;94;13M"), true},
		{"sgr wheel-up full", []rune("[<64;1;1M"), true},
		{"sgr wheel-down with shift", []rune("[<69;94;13M"), true},
		{"sgr release", []rune("[<35;10;10m"), true},
		{"sgr without leading bracket", []rune("<65;94;13M"), true},
		{"sgr partial mid-body (no terminator)", []rune("[<65;94;"), true},
		{"sgr partial no bracket", []rune("<65;94"), true},

		{"plain text typing", []rune("hello"), false},
		{"single bracket alone", []rune("["), false},
		{"too short", []rune("[<1"), false},
		{"lt with text but no digits", []rune("<abc"), false},
		{"bracket without lt", []rune("[abc]"), false},
		{"sgr-like with letters mixed in", []rune("[<65;9a;13M"), false},
		{"only semicolons after lt", []rune("<;;;"), false},
		{"angle bracket runs", []rune("<<<<"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			km := tea.KeyMsg{Type: tea.KeyRunes, Runes: c.runes}
			if got := isLeakedMouseSeq(km); got != c.want {
				t.Errorf("isLeakedMouseSeq(%q) = %v, want %v", string(c.runes), got, c.want)
			}
		})
	}
}

func TestIsLeakedMouseSeq_AltBracketOrphan(t *testing.T) {
	// alt+'[' is what bubbletea emits when the read boundary falls
	// right after the ESC byte of a mouse sequence — see comment on
	// isLeakedMouseSeq.
	km := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}, Alt: true}
	if !isLeakedMouseSeq(km) {
		t.Errorf("alt+'[' should be recognized as a leaked mouse sequence orphan")
	}

	// Plain '[' typed by the user must still pass through.
	plain := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}}
	if isLeakedMouseSeq(plain) {
		t.Errorf("plain '[' keystroke must not be filtered")
	}

	// alt+ other rune must not be filtered (no false positives on
	// real alt-key bindings).
	altA := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}, Alt: true}
	if isLeakedMouseSeq(altA) {
		t.Errorf("alt+'a' should not be filtered")
	}
}

func TestIsLeakedMouseSeq_NonRunesIgnored(t *testing.T) {
	km := tea.KeyMsg{Type: tea.KeyEnter}
	if isLeakedMouseSeq(km) {
		t.Errorf("non-KeyRunes message should never be flagged as leak")
	}
}
