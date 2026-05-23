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
	"strings"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
)

// visualState tracks the active selection. anchor and cursor are
// indices into App.fullLines.
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

// enterVisual starts visual selection at the top visible line of the
// viewport. Caller is responsible for ensuring we're transitioning
// from Normal mode.
func (a *App) enterVisual() tea.Cmd {
	a.vis.active = true
	start := a.vp.YOffset
	if start >= len(a.fullLines) {
		start = max0(len(a.fullLines) - 1)
	}
	a.vis.anchor = start
	a.vis.cursor = start
	cmd := a.setMode(modeVisual)
	a.refreshView()
	return cmd
}

// exitVisual leaves visual mode without yanking. Highlight clears on
// the next refreshView.
func (a *App) exitVisual() tea.Cmd {
	a.vis.active = false
	cmd := a.setMode(modeNormal)
	a.refreshView()
	return cmd
}

// applyVisualHighlight returns content with reverse-video styling on
// the selected range. If visual mode is inactive, returns content
// unchanged.
func (a *App) applyVisualHighlight(lines []string) []string {
	if !a.vis.active || len(lines) == 0 {
		return lines
	}
	lo, hi := a.vis.anchor, a.vis.cursor
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

// handleVisualKey dispatches keys while in Visual mode. Always
// intercepts — visual mode is modal.
func (a *App) handleVisualKey(km tea.KeyMsg) (tea.Cmd, bool) {
	switch km.String() {
	case "esc", "q":
		return a.exitVisual(), true
	case "j", "down":
		a.moveCursor(1)
	case "k", "up":
		a.moveCursor(-1)
	case "ctrl+d":
		a.moveCursor(a.vp.Height / 2)
	case "ctrl+u":
		a.moveCursor(-a.vp.Height / 2)
	case "g":
		a.vis.cursor = 0
		a.vp.GotoTop()
		a.refreshView()
	case "G":
		a.vis.cursor = max0(len(a.fullLines) - 1)
		a.vp.GotoBottom()
		a.refreshView()
	case "y", "enter":
		return a.yankVisualSelection(), true
	case "o":
		// Vim convention: 'o' swaps anchor and cursor so the user can
		// extend selection from the other end.
		a.vis.anchor, a.vis.cursor = a.vis.cursor, a.vis.anchor
		a.refreshView()
	}
	return nil, true
}

// moveCursor advances the visual cursor by delta lines and scrolls
// the viewport so the cursor stays visible.
func (a *App) moveCursor(delta int) {
	if len(a.fullLines) == 0 {
		return
	}
	a.vis.cursor += delta
	if a.vis.cursor < 0 {
		a.vis.cursor = 0
	}
	if a.vis.cursor >= len(a.fullLines) {
		a.vis.cursor = len(a.fullLines) - 1
	}
	// Keep cursor inside the viewport window. YOffset is the top
	// visible line; YOffset+Height-1 is the bottom visible line.
	top := a.vp.YOffset
	bottom := top + a.vp.Height - 1
	if a.vis.cursor < top {
		a.vp.SetYOffset(a.vis.cursor)
	} else if a.vis.cursor > bottom {
		a.vp.SetYOffset(a.vis.cursor - a.vp.Height + 1)
	}
	a.refreshView()
}

// yankVisualSelection copies the [anchor, cursor] line range to the
// system clipboard (OSC 52), ANSI-stripped. Logs an info line with
// the byte count and exits visual mode.
func (a *App) yankVisualSelection() tea.Cmd {
	lo, hi := a.vis.anchor, a.vis.cursor
	if lo > hi {
		lo, hi = hi, lo
	}
	if lo < 0 {
		lo = 0
	}
	if hi >= len(a.fullLines) {
		hi = len(a.fullLines) - 1
	}
	if lo > hi {
		return a.exitVisual()
	}
	plain := stripANSI(strings.Join(a.fullLines[lo:hi+1], "\n"))
	yankCmd := yankToClipboardCmd(plain)
	lineCount := hi - lo + 1
	a.vis.active = false
	modeCmd := a.setMode(modeNormal)
	a.appendItem(chatItem{
		kind: itemInfo,
		text: fmt.Sprintf("yanked %d bytes (%d lines) to clipboard", len(plain), lineCount),
	})
	a.refreshView()
	return tea.Batch(yankCmd, modeCmd)
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
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
func (a *App) handleMouse(m tea.MouseMsg) tea.Cmd {
	// Modal modes ignore selection mouse: the permission card has its
	// own focus model; the pager handles its own scroll.
	if a.mode == modePermission || a.mode == modePager {
		return nil
	}

	// Wheel events go straight to the viewport via vp.Update below;
	// only the left button drives selection.
	if m.Button != tea.MouseButtonLeft {
		return nil
	}

	// The viewport occupies screen rows [0, vp.Height-1]. Anything
	// outside that band is chrome / permission / status / input — not
	// selectable.
	if m.Y < 0 || m.Y >= a.vp.Height {
		return nil
	}

	// Map screen-row → line index in fullLines. This assumes one
	// fullLines entry per visual row, which holds because chatItem
	// rendering pre-wraps content to width.
	line := a.vp.YOffset + m.Y
	if line < 0 {
		line = 0
	}
	if line >= len(a.fullLines) {
		line = max0(len(a.fullLines) - 1)
	}

	switch m.Action {
	case tea.MouseActionPress:
		a.vis.active = true
		a.vis.anchor = line
		a.vis.cursor = line
		cmd := a.setMode(modeVisual)
		a.refreshView()
		return cmd

	case tea.MouseActionMotion:
		if !a.vis.active {
			return nil
		}
		a.vis.cursor = line
		// Auto-scroll when the drag pointer is near the viewport
		// edge. One row of slack on each side avoids juddering as
		// the cursor crosses the boundary.
		const edge = 0
		if m.Y <= edge && a.vp.YOffset > 0 {
			a.vp.SetYOffset(a.vp.YOffset - 1)
		} else if m.Y >= a.vp.Height-1-edge &&
			a.vp.YOffset+a.vp.Height < len(a.fullLines) {
			a.vp.SetYOffset(a.vp.YOffset + 1)
		}
		a.refreshView()
		return nil

	case tea.MouseActionRelease:
		if !a.vis.active {
			return nil
		}
		// If anchor == cursor it was a plain click, not a drag — bail
		// without yanking so users don't get spurious clipboard writes.
		if a.vis.anchor == a.vis.cursor {
			return a.exitVisual()
		}
		return a.yankVisualSelection()
	}
	return nil
}

