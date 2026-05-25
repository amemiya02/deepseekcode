// visual.go implements a Vim-style line-range selection mode for the
// scrollback. Activated by `v` in Normal mode.
//
// Why we need this: alt-screen mode (tea.WithAltScreen) means the
// terminal shows only the currently rendered frame — no scrollback
// history above it. Terminal-native shift-drag selection therefore
// can't reach content that the user has scrolled past, which is
// exactly the case for long agent outputs that span multiple screens.
//
// Visual mode solves that by tracking a line-space cursor that:
//   - moves with j/k, ^U/^D, gg/G inside the in-app viewport
//   - scrolls the viewport when it crosses the visible window
//   - paints a reverse-video highlight on the [anchor, cursor] range
//   - yanks the highlighted text (ANSI-stripped) on `y`
//
// Selection survives content updates because indices live in
// "rendered line" space — newly appended lines extend the buffer but
// don't shift existing indices.
package tui

import (
	"fmt"
	"regexp"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// visualState tracks the active selection. anchor and cursor are
// indices into Scrollback.fullLines. Lives in scrollback.go's module
// but is named here to keep the selection-rendering helpers
// (applyVisualHighlightLines, stripANSI) self-contained.
type visualState struct {
	active bool
	anchor int
	cursor int
}

// ansiCSI matches CSI escape sequences (`\x1b[...m`, cursor moves,
// etc). OSC sequences are not stripped because none should appear in
// rendered content — OSC 52 is written separately to stderr.
var ansiCSI = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

// stripANSI removes CSI escape sequences from s. Used when yanking a
// selection — the user wants the visible text, not the styling.
func stripANSI(s string) string {
	return ansiCSI.ReplaceAllString(s, "")
}

// applyVisualHighlightLines returns lines with reverse-video styling
// applied to the [anchor, cursor] range. Stand-alone (not a method)
// so Scrollback.Render can call it without circular deps.
func applyVisualHighlightLines(lines []string, vis visualState) []string {
	if !vis.active || len(lines) == 0 {
		return lines
	}
	lo, hi := vis.anchor, vis.cursor
	if lo > hi {
		lo, hi = hi, lo
	}
	if lo < 0 {
		lo = 0
	}
	if hi >= len(lines) {
		hi = len(lines) - 1
	}
	highlight := lipgloss.NewStyle().Reverse(true)
	out := make([]string, len(lines))
	copy(out, lines)
	for i := lo; i <= hi; i++ {
		// Strip existing styles so reverse video isn't double-applied
		// — the styling collisions otherwise produce flicker on terms
		// that don't merge nested SGR cleanly.
		out[i] = highlight.Render(stripANSI(out[i]))
	}
	return out
}

// enterVisual starts visual selection at the top visible line of the
// viewport. Caller is responsible for ensuring we're transitioning
// from Normal mode.
func (a *App) enterVisual() tea.Cmd {
	a.scrollback.BeginSelection(a.vp.YOffset())
	cmd := a.setMode(modeVisual)
	a.refreshView()
	return cmd
}

// exitVisual leaves visual mode without yanking. Highlight clears on
// the next refreshView.
func (a *App) exitVisual() tea.Cmd {
	a.scrollback.CancelSelection()
	cmd := a.setMode(modeNormal)
	a.refreshView()
	return cmd
}

// handleVisualKey dispatches keys while in Visual mode. Always
// intercepts — visual mode is modal.
func (a *App) handleVisualKey(km tea.KeyPressMsg) (tea.Cmd, bool) {
	switch km.String() {
	case "esc", "q":
		return a.exitVisual(), true
	case "j", "down":
		a.moveCursor(1)
	case "k", "up":
		a.moveCursor(-1)
	case "ctrl+d":
		a.moveCursor(a.vp.Height() / 2)
	case "ctrl+u":
		a.moveCursor(-a.vp.Height() / 2)
	case "g":
		a.scrollback.ExtendSelection(0)
		a.vp.GotoTop()
		a.refreshView()
	case "G":
		a.scrollback.ExtendSelection(len(a.scrollback.FullLines()) - 1)
		a.vp.GotoBottom()
		a.refreshView()
	case "y", "enter":
		return a.yankVisualSelection(), true
	case "o":
		// Vim convention: 'o' swaps anchor and cursor so the user can
		// extend selection from the other end.
		a.scrollback.SwapSelectionEnds()
		a.refreshView()
	}
	return nil, true
}

// moveCursor advances the visual cursor by delta lines and scrolls
// the viewport so the cursor stays visible.
func (a *App) moveCursor(delta int) {
	lines := a.scrollback.FullLines()
	if len(lines) == 0 {
		return
	}
	cursor := a.scrollback.SelectionCursor() + delta
	a.scrollback.ExtendSelection(cursor)
	// Re-read post-clamp so the viewport math uses the actual cursor.
	cursor = a.scrollback.SelectionCursor()
	// Keep cursor inside the viewport window. YOffset is the top
	// visible line; YOffset+Height-1 is the bottom visible line.
	top := a.vp.YOffset()
	bottom := top + a.vp.Height() - 1
	if cursor < top {
		a.vp.SetYOffset(cursor)
	} else if cursor > bottom {
		a.vp.SetYOffset(cursor - a.vp.Height() + 1)
	}
	a.refreshView()
}

// yankVisualSelection copies the [anchor, cursor] line range to the
// system clipboard (OSC 52), ANSI-stripped. Logs an info line with
// the byte count and exits visual mode.
func (a *App) yankVisualSelection() tea.Cmd {
	lo, hi, active := a.scrollback.SelectionRange()
	if !active {
		return a.exitVisual()
	}
	plain := a.scrollback.EndSelection()
	if plain == "" {
		modeCmd := a.setMode(modeNormal)
		a.refreshView()
		return modeCmd
	}
	yankCmd := yankToClipboardCmd(plain)
	lineCount := hi - lo + 1
	modeCmd := a.setMode(modeNormal)
	a.scrollback.AppendInfo(fmt.Sprintf("yanked %d bytes (%d lines) to clipboard", len(plain), lineCount))
	a.refreshView()
	return tea.Batch(yankCmd, modeCmd)
}

// handleMouse drives mouse-based visual selection. Mouse capture is
// enabled in Run(), so click/drag/release events arrive here.
//
// Semantics:
//   - Left-click → start visual selection at the clicked line
//   - Drag      → extend the selection cursor to the dragged line
//   - Drag near top/bottom edge of viewport → auto-scroll, so the
//     selection can span content that was scrolled past
//   - Release   → yank the selected text to clipboard (OSC 52) and
//     exit visual mode
//
// Returns the tea.Cmd to execute (nil for press/drag, the yank cmd
// for release).
type mouseAction int

const (
	mouseActionPress mouseAction = iota
	mouseActionMotion
	mouseActionRelease
)

// isV2MouseMsg reports whether msg is one of the four v2 mouse message types.
func isV2MouseMsg(msg tea.Msg) bool {
	switch msg.(type) {
	case tea.MouseClickMsg, tea.MouseMotionMsg, tea.MouseReleaseMsg, tea.MouseWheelMsg:
		return true
	}
	return false
}

func (a *App) handleMouse(m tea.Mouse, action mouseAction) tea.Cmd {
	// Modal modes ignore selection mouse: the permission card has its
	// own focus model; the pager handles its own scroll.
	if a.mode == modePermission || a.mode == modePager {
		return nil
	}

	// Wheel events go straight to the viewport via vp.Update below;
	// only the left button drives selection.
	if m.Button != tea.MouseLeft {
		return nil
	}

	// The viewport occupies screen rows [0, vp.Height-1]. Anything
	// outside that band is chrome / permission / status / input — not
	// selectable.
	if m.Y < 0 || m.Y >= a.vp.Height() {
		return nil
	}

	// Map screen-row → line index in fullLines. This assumes one
	// fullLines entry per visual row, which holds because chatItem
	// rendering pre-wraps content to width.
	lines := a.scrollback.FullLines()
	line := a.vp.YOffset() + m.Y

	// Dispatch by message type: click=start, motion=extend, release=end.
	switch action {
	case mouseActionPress:
		a.scrollback.BeginSelection(line)
		cmd := a.setMode(modeVisual)
		a.refreshView()
		return cmd

	case mouseActionMotion:
		if !a.scrollback.SelectionActive() {
			return nil
		}
		a.scrollback.ExtendSelection(line)
		// Auto-scroll when the drag pointer is near the viewport
		// edge. One row of slack on each side avoids juddering as
		// the cursor crosses the boundary.
		const edge = 0
		if m.Y <= edge && a.vp.YOffset() > 0 {
			a.vp.SetYOffset(a.vp.YOffset() - 1)
		} else if m.Y >= a.vp.Height()-1-edge &&
			a.vp.YOffset()+a.vp.Height() < len(lines) {
			a.vp.SetYOffset(a.vp.YOffset() + 1)
		}
		a.refreshView()
		return nil

	case mouseActionRelease:
		if !a.scrollback.SelectionActive() {
			return nil
		}
		lo, hi, _ := a.scrollback.SelectionRange()
		// If anchor == cursor it was a plain click, not a drag — bail
		// without yanking so users don't get spurious clipboard writes.
		if lo == hi {
			return a.exitVisual()
		}
		return a.yankVisualSelection()
	}
	return nil
}
