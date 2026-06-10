package tui

import (
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/i18n"
)

// The completions popup FLOATS over the transcript band (overlayPopup) at a
// height fixed per trigger session, so opening, filtering, or closing it never
// reflows the frame: the header, the transcript geometry, the chrome/divider/
// status HUD cluster, the input box, and the hint all keep their terminal rows.
// These tests pin that anchoring, plus the terminal-height ceiling that keeps
// the card off the header and the input on short terminals.

// renderedView is the App's View content as a slice of lines.
func renderedView(a *App) []string {
	return strings.Split(a.View().Content, "\n")
}

// promptRow returns the index of the first frame line containing the input
// prompt glyph, or -1 when the input box is missing from the frame.
func promptRow(lines []string) int {
	for i, l := range lines {
		if strings.Contains(l, "›") {
			return i
		}
	}
	return -1
}

// TestPopupClampedOnShortTerminalKeepsInputVisible drives the `/` menu open on
// a short terminal — tall enough that the chrome + the 5-row input box (3
// textarea rows + 2 border) + a minimum transcript fit, but short enough that
// the popup's full 12-line card would overrun the transcript band it floats
// over — and asserts the card is clamped (fewer than its 10 rows), the View
// fits the height, and the input prompt glyph survives. At height 16 the
// transcript band is 6 rows, so the card gets at most 4 body rows; without
// the clamp it would reach past the header.
func TestPopupClampedOnShortTerminalKeepsInputVisible(t *testing.T) {
	const h = 16
	a := sizeApp(t, newKeyflowApp(t), 80, h)
	baseline := renderedView(a)

	// Type '/' in Insert mode: it falls through to the textarea and
	// syncCompletions opens the `/` menu over the built-in command set.
	a = drive(t, a, press('/'))
	if !a.completions.Active() {
		t.Fatal("typing '/' at a word boundary should open the completions popup")
	}

	// The clamp must have engaged: the popup wants its full 10-row built-in set
	// but the transcript band only spares a few rows, so visibleRows is reduced.
	// Still at least one row (the popup is visible, not suppressed) at this height.
	vr := a.completions.visibleRows()
	if vr <= 0 || vr >= 10 {
		t.Fatalf("visibleRows = %d on a %d-row terminal, want it clamped into (0,10)", vr, h)
	}

	lines := renderedView(a)
	if len(lines) > a.height {
		t.Fatalf("View rendered %d lines on a %d-row terminal — the popup overflowed the stack:\n%s",
			len(lines), a.height, a.View().Content)
	}

	// The input prompt glyph must still be present: the floating card must
	// never cover the input box.
	if promptRow(lines) < 0 {
		t.Fatalf("input prompt glyph missing — the popup covered the input box:\n%s", a.View().Content)
	}

	// The card must also stay off the header row (the overlay's top guard).
	// The header is gradient-styled per rune, so compare against the
	// pre-popup baseline rather than searching for a literal substring.
	if lines[0] != baseline[0] {
		t.Fatalf("header row covered by the popup on a %d-row terminal:\n%s", h, a.View().Content)
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

// TestPopupOverlayKeepsFrameAnchored is the regression test for the popup
// bouncing the frame: opening the `/` menu, filtering it down to fewer (then
// zero) matches, and closing it must never move the header, the transcript
// top, the chrome/divider/status HUD cluster, the input box, or the hint —
// and the card's height must stay fixed for the whole session.
func TestPopupOverlayKeepsFrameAnchored(t *testing.T) {
	const w, h = 80, 30
	a := sizeApp(t, newKeyflowApp(t), w, h)
	before := renderedView(a)
	if len(before) != h {
		t.Fatalf("baseline frame = %d lines, want %d", len(before), h)
	}
	// Frame anatomy at this size (no mode chip, no toast): hint is the last
	// row, the 5-row input box (3 textarea rows + 2 border) sits above it, and
	// the chrome/divider/status cluster sits above that. The popup floats over
	// the transcript band only.
	inputTop := h - 1 - 5
	anchored := func() []int {
		rows := []int{0, 1, 2} // header + transcript top
		for i := inputTop - 3; i < inputTop; i++ {
			rows = append(rows, i) // chrome, divider, status HUD
		}
		return append(rows, h-1) // hint
	}

	a = drive(t, a, press('/'))
	after := renderedView(a)
	if len(after) != len(before) {
		t.Fatalf("opening the menu changed the frame height: %d -> %d lines", len(before), len(after))
	}
	for _, i := range anchored() {
		if after[i] != before[i] {
			t.Fatalf("opening the menu moved anchored row %d:\nbefore: %q\nafter:  %q", i, before[i], after[i])
		}
	}
	if pr := promptRow(after); pr != promptRow(before) {
		t.Fatalf("opening the menu moved the input prompt: row %d -> %d", promptRow(before), pr)
	}
	if a.popupLines != 12 {
		t.Fatalf("popupLines after open = %d, want 12 (10 built-ins + 2 borders)", a.popupLines)
	}

	// Filter down ('m' narrows the set), then to zero matches ('z','z'): the
	// card height and every anchored row must not move on any keystroke.
	for _, r := range []rune{'m', 'z', 'z'} {
		a = drive(t, a, press(r))
		if a.popupLines != 12 {
			t.Fatalf("popupLines after typing %q = %d, want it fixed at 12", r, a.popupLines)
		}
		cur := renderedView(a)
		if len(cur) != h {
			t.Fatalf("frame height after typing %q = %d lines, want %d", r, len(cur), h)
		}
		for _, i := range anchored() {
			if cur[i] != before[i] {
				t.Fatalf("typing %q moved anchored row %d:\nbefore: %q\nafter:  %q", r, i, before[i], cur[i])
			}
		}
	}
	// "/mzz" matches nothing: the card shows the dimmed notice, not a collapse.
	if !strings.Contains(a.View().Content, i18n.T("app.completions.none")) {
		t.Fatalf("zero-match menu should show the %q notice:\n%s", i18n.T("app.completions.none"), a.View().Content)
	}

	// Closing the menu restores the frame exactly, except the input box rows
	// (the typed "/mzz" draft legitimately remains in the textarea).
	a = drive(t, a, keyEscape())
	if a.completions.Active() {
		t.Fatal("esc should close the menu")
	}
	if a.popupLines != 0 {
		t.Fatalf("popupLines after close = %d, want 0", a.popupLines)
	}
	closed := renderedView(a)
	if len(closed) != h {
		t.Fatalf("frame height after close = %d lines, want %d", len(closed), h)
	}
	for i := range closed {
		if i >= inputTop && i < inputTop+5 {
			continue // input box rows hold the typed draft
		}
		if closed[i] != before[i] {
			t.Fatalf("closing the menu left row %d altered:\nbefore: %q\nafter:  %q", i, before[i], closed[i])
		}
	}
}
