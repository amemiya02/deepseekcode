package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestSetCursorByteOffsetRowCol unit-tests the (row, col) derivation behind the
// multi-line caret fix, including a multibyte rune so the byte→rune column
// conversion is exercised.
func TestSetCursorByteOffsetRowCol(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	a.input.SetValue("héllo\nworld")

	// Offset 9 = "héllo"(6 bytes) + "\n"(1) + "wo"(2): row 1, col 2 (runes).
	a.setCursorByteOffset(9)
	if a.input.Line() != 1 || a.input.Column() != 2 {
		t.Fatalf("offset 9 → caret (row %d, col %d), want (1, 2)", a.input.Line(), a.input.Column())
	}
	// Offset 3 = after "hé" on row 0 ("h"=1 byte, "é"=2 bytes): row 0, col 2.
	a.setCursorByteOffset(3)
	if a.input.Line() != 0 || a.input.Column() != 2 {
		t.Fatalf("offset 3 → caret (row %d, col %d), want (0, 2)", a.input.Line(), a.input.Column())
	}
}

// TestAcceptCompletionCaretAcrossRows pins the multi-line caret fix: accepting a
// completion whose prefix sits on an upper row of a multi-line draft must land
// the caret right after the inserted text on the CORRECT row. The old
// SetCursorColumn-only path set the column on whatever row SetValue left the
// caret on (the last row), so the caret jumped to the wrong line.
func TestAcceptCompletionCaretAcrossRows(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)

	a.input.SetValue("/mo\nsecond line")
	a.setCursorByteOffset(3) // caret just after "/mo" on row 0
	a.completions.Open('/', []complItem{{insert: "/models", label: "/models"}}, 0)

	a.acceptCompletion()

	if got, want := a.input.Value(), "/models\nsecond line"; got != want {
		t.Fatalf("accepted value = %q, want %q", got, want)
	}
	if a.input.Line() != 0 {
		t.Fatalf("caret row = %d, want 0 (must stay on the accepted token's row, not jump to the last row)", a.input.Line())
	}
	if want := len([]rune("/models")); a.input.Column() != want {
		t.Fatalf("caret col = %d, want %d (end of the inserted text)", a.input.Column(), want)
	}
}

// TestHistoryRecallRederivesPopup: recalling a prior prompt that begins with a
// trigger ('/') re-opens the inline menu. The popup is a pure mirror of the
// buffer, and the intercepted ↑ path must derive it explicitly (it skips
// Update's trailing sync).
func TestHistoryRecallRederivesPopup(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	a.history = newPromptHistory([]string{"/models"}, 500)
	a = drive(t, a, tea.KeyPressMsg{Code: tea.KeyUp})
	if got := a.input.Value(); got != "/models" {
		t.Fatalf("↑ should recall the newest entry; input = %q, want %q", got, "/models")
	}
	if !a.completions.Active() || a.completions.Trigger() != '/' {
		t.Fatalf("recalling a /-entry must re-open the menu; active=%v trigger=%q",
			a.completions.Active(), a.completions.Trigger())
	}

	// A recalled plain prompt leaves the menu closed (the same buffer-mirror
	// invariant, the other way). Fresh app: with the menu open from above, ↑
	// would navigate the menu rather than recall (popup precedence, §7), so the
	// close path is only reachable from a closed-menu start — which is exactly
	// when a plain prior prompt is recalled.
	b := sizeApp(t, newKeyflowApp(t), 100, 40)
	b.history = newPromptHistory([]string{"plain prompt"}, 500)
	b = drive(t, b, tea.KeyPressMsg{Code: tea.KeyUp})
	if got := b.input.Value(); got != "plain prompt" {
		t.Fatalf("↑ should recall the entry; input = %q", got)
	}
	if b.completions.Active() {
		t.Fatalf("recalling a non-trigger entry must not open the menu")
	}
}
