// queue_paste.go owns two P2 input affordances:
//
//   - G11 prompt queueing: a prompt submitted while a run is active is appended
//     to a.queued instead of starting a second run; the active run's Done path
//     drains the next queued prompt and submits it. The queued count is surfaced
//     subtly via a toast on enqueue and on the status line.
//   - G12 large-paste collapse: a bracketed paste over pasteCollapseLines lines
//     is held in full in a.pasteStore and the DISPLAY collapses to a one-line
//     "[pasted N lines]" chip. The chip is expanded back to the full text at
//     submit time, so the model always sees the real paste while the input box
//     stays a single readable line.
package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// pasteCollapseLines is the line count at which a single paste collapses to a
// chip. Below it a paste is small enough to read inline, so it is left
// untouched; at or above it the wall-of-text is hidden behind the chip.
const pasteCollapseLines = 6

// queueDepth reports how many prompts are waiting behind the active run (G11).
func (a *App) queueDepth() int { return len(a.queued) }

// enqueuePrompt appends a prompt submitted while a run is active to the pending
// queue and returns the toast Cmd announcing the new depth. The text is the
// already-expanded prompt (chips resolved), so the drained entry is submitted
// verbatim with no further processing.
func (a *App) enqueuePrompt(text string) tea.Cmd {
	a.queued = append(a.queued, text)
	a.status.queued = len(a.queued)
	a.scrollback.AppendUser(text)
	a.refreshView()
	return a.toast(BadgeInfo, fmt.Sprintf("queued (%d pending)", len(a.queued)))
}

// drainQueue pops the oldest queued prompt and returns the Cmd that submits it,
// or nil when the queue is empty. Called on the Done path so a finished run
// flows straight into the next queued prompt. The prompt was already echoed to
// the scrollback when it was enqueued, so drain only resubmits it.
func (a *App) drainQueue() tea.Cmd {
	if len(a.queued) == 0 {
		return nil
	}
	next := a.queued[0]
	a.queued = a.queued[1:]
	a.status.queued = len(a.queued)
	return a.submitPromptCmd(next)
}

// clearQueue drops every pending prompt and returns a toast Cmd reporting how
// many were discarded, or nil when the queue was already empty. Called when the
// user cancels the active run (^C): a cancel is an explicit "stop", so the
// queued follow-ups are dropped rather than silently run after the cancel.
func (a *App) clearQueue() tea.Cmd {
	n := len(a.queued)
	if n == 0 {
		return nil
	}
	a.queued = nil
	a.status.queued = 0
	return a.toast(BadgeWarn, fmt.Sprintf("cancelled — dropped %d queued prompt(s)", n))
}

// chipPlaceholder formats the collapsed-paste chip shown in the input for a
// paste of n lines. It is a single line so the input box stays compact; the
// full text lives in a.pasteStore keyed by this exact string.
func chipPlaceholder(n int) string {
	return fmt.Sprintf("[pasted %d lines]", n)
}

// handlePaste collapses a large bracketed paste (G12). A paste spanning fewer
// than pasteCollapseLines lines is inserted verbatim (returns false so the
// caller forwards the PasteMsg to the textarea unchanged). A larger paste is
// stored in full under a unique chip token, the chip is inserted into the input
// in its place, and true is returned so the caller does NOT also forward the raw
// paste. Multiple large pastes coexist: each gets its own chip + store entry.
func (a *App) handlePaste(content string) bool {
	if a.mode != modeInsert {
		return false // only collapse pastes into the live prompt
	}
	lines := strings.Count(content, "\n") + 1
	if lines < pasteCollapseLines {
		return false
	}
	if a.pasteStore == nil {
		a.pasteStore = make(map[string]string)
	}
	// Make the chip unique even when two pastes have the same line count, so two
	// chips never collapse into one store entry. The counter is monotonic per
	// App and never reused within a session.
	a.pasteSeq++
	chip := chipPlaceholder(lines)
	if a.pasteSeq > 1 {
		// Disambiguate repeat counts with a trailing marker the expander strips.
		chip = fmt.Sprintf("[pasted %d lines #%d]", lines, a.pasteSeq)
	}
	a.pasteStore[chip] = content
	a.input.InsertString(chip)
	return true
}

// expandPaste replaces every paste chip in text with its stored full content
// (G12), so the submitted prompt carries the real paste even though the input
// box only showed the chip. Chips not present in the store (e.g. a chip the
// user hand-typed) are left as-is. Returns the expanded text; it does not clear
// the store — resetPasteState does that after the buffer is consumed.
func (a *App) expandPaste(text string) string {
	if len(a.pasteStore) == 0 {
		return text
	}
	for chip, full := range a.pasteStore {
		text = strings.ReplaceAll(text, chip, full)
	}
	return text
}

// resetPasteState clears the collapsed-paste store after the input buffer is
// consumed (submit or reset) so chips from a previous prompt can never leak
// into the next one. The seq counter is left alone so chip tokens stay unique
// across the whole session.
func (a *App) resetPasteState() {
	a.pasteStore = nil
}
