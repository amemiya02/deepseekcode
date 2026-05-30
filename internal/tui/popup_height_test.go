package tui

import (
	"strings"
	"testing"
)

// The completions popup grows upward from the prompt. On a short terminal its
// full 10-row window (12 lines with borders) plus the fixed chrome/status/input
// rows used to overflow the View stack and push the input box off the bottom of
// the screen. layout() now caps the popup against the terminal height; these
// tests pin that the input box always survives and the View never overflows.

// renderedView is the App's View content as a slice of lines.
func renderedView(a *App) []string {
	return strings.Split(a.View().Content, "\n")
}

// TestPopupClampedOnShortTerminalKeepsInputVisible drives the `/` menu open on a
// short terminal — tall enough that the chrome + input + a minimum body fit, but
// short enough that the popup's full 12-line window would overflow — and asserts
// the popup is clamped (fewer than its 10 rows), the View fits the height, and
// the input prompt glyph survives. height 20 is the regression point: without
// the clamp the popup would push the input box off the bottom of the screen.
func TestPopupClampedOnShortTerminalKeepsInputVisible(t *testing.T) {
	const h = 20
	a := sizeApp(t, newKeyflowApp(t), 80, h)

	// Type '/' in Insert mode: it falls through to the textarea and
	// syncCompletions opens the `/` menu over the built-in command set.
	a = drive(t, a, press('/'))
	if !a.completions.Active() {
		t.Fatal("typing '/' at a word boundary should open the completions popup")
	}

	// The clamp must have engaged: the popup wants its full 10-row built-in set
	// but the terminal only spares a few rows, so visibleRows is reduced. Still
	// at least one row (the popup is visible, not suppressed) at this height.
	vr := a.completions.visibleRows()
	if vr <= 0 || vr >= 10 {
		t.Fatalf("visibleRows = %d on a %d-row terminal, want it clamped into (0,10)", vr, h)
	}

	lines := renderedView(a)
	if len(lines) > a.height {
		t.Fatalf("View rendered %d lines on a %d-row terminal — the popup overflowed the stack:\n%s",
			len(lines), a.height, a.View().Content)
	}

	// The input prompt glyph must still be present: if the popup pushed the
	// input box off the bottom, the glyph would be gone.
	if !strings.Contains(a.View().Content, "›") {
		t.Fatalf("input prompt glyph missing — the popup pushed the input box off-screen:\n%s", a.View().Content)
	}

	// The reserved popup height must leave room for the fixed rows + a minimum
	// body, never claiming the whole screen.
	if a.popupLines >= a.height {
		t.Fatalf("popupLines = %d on a %d-row terminal — the reserve must be bounded by height", a.popupLines, a.height)
	}
}

// TestPopupFullSizeOnTallTerminal verifies the clamp is inert when there is
// ample room: the popup shows its full window and the View still fits.
func TestPopupFullSizeOnTallTerminal(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 80, 40)
	a = drive(t, a, press('/'))
	if !a.completions.Active() {
		t.Fatal("typing '/' should open the completions popup")
	}
	// With 10 built-in commands the popup wants all of them (10 rows + 2
	// borders). On a tall terminal the cap must not reduce that.
	if got := a.completions.visibleRows(); got != 10 {
		t.Fatalf("visibleRows on a tall terminal = %d, want 10 (full built-in set)", got)
	}
	if a.popupLines != 12 {
		t.Fatalf("popupLines on a tall terminal = %d, want 12 (10 rows + 2 borders)", a.popupLines)
	}
	if lines := renderedView(a); len(lines) > a.height {
		t.Fatalf("View rendered %d lines on a %d-row terminal", len(lines), a.height)
	}
}
