package tui

import tea "github.com/charmbracelet/bubbletea"

// isLeakedMouseSeq reports whether km is an SGR mouse escape sequence
// that Bubble Tea v1.3.10 failed to recognize and instead emitted as a
// KeyMsg full of runes — i.e. an event that should be silently dropped
// rather than typed into the textarea.
//
// Why this happens: readAnsiInputs (bubbletea key.go:556) reads stdin
// in 256-byte chunks. The SGR mouse detector at key.go:618 requires
// the buffer to start with "\x1b[<" before it will attempt a match.
// When the user spins the wheel quickly (especially while the viewport
// is already at the bottom and the scroll is a visual no-op, so they
// keep scrolling), many sequences pile up in the OS pipe. If the
// 256-byte read boundary lands in the middle of a sequence, the second
// half arrives in the next read WITHOUT its leading ESC. The mouse
// detector skips it, the rune-gather path (key.go:670-685) sweeps up
// the remaining ASCII bytes as a single KeyRunes KeyMsg, and the
// textarea cheerfully renders "[<65;94;13M" as user input.
//
// Legitimate typing always arrives as one rune per KeyMsg — only paste
// or stream events produce multi-rune KeyRunes, and bracketed paste is
// handled as PasteMsg before reaching us. So any multi-rune KeyMsg
// whose runes look like an SGR mouse sequence is, in practice, always
// a leak.
//
// The match accepts:
//   - "[<digits;digits;digits[Mm]"  — common case, ESC stripped
//   - "<digits;digits;digits[Mm]"   — when '[' was already emitted as a
//     separate alt+'[' KeyMsg (see below)
//   - either of the above without the trailing M/m (partial leak when
//     the read boundary fell inside the sequence body)
//   - alt+'[' single-rune KeyMsg — produced when the read boundary
//     falls right after the ESC. bubbletea's parser then sees only
//     "\x1b[" in the chunk, can't match mouse (needs >= 6 bytes),
//     can't match a known CSI sequence, so it treats ESC as the alt
//     modifier and emits "[" as the rune. The textarea's default case
//     ignores the alt flag and inserts the rune as text (bubbles
//     textarea.go:1065), which is the source of the lone "[" garbage.
//     No app binding uses alt+'[' and macOS already sends alt as an
//     ESC prefix, so this can only originate from a split mouse
//     sequence — safe to swallow.
func isLeakedMouseSeq(km tea.KeyMsg) bool {
	if km.Type != tea.KeyRunes {
		return false
	}
	if km.Alt && len(km.Runes) == 1 && km.Runes[0] == '[' {
		return true
	}
	if len(km.Runes) < 4 {
		return false
	}
	s := km.Runes
	switch {
	case len(s) >= 2 && s[0] == '[' && s[1] == '<':
		s = s[2:]
	case s[0] == '<':
		s = s[1:]
	default:
		return false
	}
	if len(s) == 0 {
		return false
	}
	end := len(s)
	if last := s[end-1]; last == 'M' || last == 'm' {
		end--
	}
	if end == 0 {
		return false
	}
	sawDigit := false
	for i := 0; i < end; i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			sawDigit = true
		case c == ';':
		default:
			return false
		}
	}
	return sawDigit
}
