// yank.go implements clipboard copy via OSC 52 escape sequences.
//
// OSC 52 ("Set Clipboard") is honored by iTerm2, kitty, alacritty,
// WezTerm, and recent tmux/screen. It is the only portable way to
// write the system clipboard from inside a TUI without taking a hard
// dependency on platform-specific paste daemons (xclip, pbcopy, wsl).
// On terminals that don't support it the sequence is silently ignored
// — no crash, no visible noise.
//
// We deliberately write to stderr rather than stdout. Bubble Tea's
// renderer owns stdout and would otherwise risk interleaving the
// escape mid-frame. stderr connects to the same tty in interactive
// sessions, so the terminal sees the sequence either way.
package tui

import (
	"encoding/base64"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// osc52MaxBytes mirrors the de-facto cap implemented by most
// terminals. Payloads above this are truncated to avoid runaway
// escape sequences corrupting the terminal state on terminals that
// don't actually enforce a length limit.
const osc52MaxBytes = 8000

// writeOSC52 emits the clipboard-set escape to stderr. No-op for empty
// input. Truncates oversized payloads.
func writeOSC52(text string) {
	if text == "" {
		return
	}
	if len(text) > osc52MaxBytes {
		text = text[:osc52MaxBytes]
	}
	b64 := base64.StdEncoding.EncodeToString([]byte(text))
	// BEL (\x07) terminator has broader compatibility than ST (\x1b\\).
	fmt.Fprintf(os.Stderr, "\x1b]52;c;%s\x07", b64)
}

// yankToClipboardCmd defers the OSC 52 write to a tea.Cmd so it runs
// at a safe point in the Bubble Tea cycle rather than mid-Update.
func yankToClipboardCmd(text string) tea.Cmd {
	return func() tea.Msg {
		writeOSC52(text)
		return nil
	}
}
