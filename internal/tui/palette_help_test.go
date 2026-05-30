package tui

import (
	"strconv"
	"strings"
	"testing"
)

// TestPaletteWindowFollowsCursorAndScrolls pins the scroll fix: with more
// actions than fit the pane, the rendered window must follow the cursor (late
// rows visible, early rows scrolled off) and a scrollbar must appear. Before
// the fix renderPalette drew every row from the top and let wrapPane clip, so
// ↓ advanced an off-screen cursor and the view never scrolled.
func TestPaletteWindowFollowsCursorAndScrolls(t *testing.T) {
	const n = 100
	actions := make([]paletteAction, n)
	visible := make([]int, n)
	for i := 0; i < n; i++ {
		label := "/cmd" + strconv.Itoa(i)
		actions[i] = paletteAction{id: label, label: label, detail: "do a thing"}
		visible[i] = i
	}
	// Cursor near the end, a short pane so the list overflows.
	out := stripANSI(renderPalette(DarkTheme(), actions, visible, 90, "", 100, 20))

	if !strings.Contains(out, "/cmd90") {
		t.Errorf("cursor row /cmd90 must be in view after scrolling; got:\n%s", out)
	}
	if strings.Contains(out, "/cmd0") {
		t.Error("top rows must scroll off the window; /cmd0 should not be visible")
	}
	if !strings.Contains(out, ScrollbarThumb) && !strings.Contains(out, ScrollbarTrack) {
		t.Error("a scrollbar should render when the action list overflows the pane")
	}
}

// These cover G5 (command palette), G6 (filterable pickers), and G7 (help
// overlay): the filter narrows rows live, the palette opens and runs an
// action, help opens and closes, and `?` stays a literal character while
// typing (Insert mode) but opens help in Normal mode.

// --- G6: filterable pickers ----------------------------------------------

// TestModelsPickerFilterNarrows: typing into the /models picker narrows the
// visible rows to fuzzy matches and the cursor selects from the narrowed set.
func TestModelsPickerFilterNarrows(t *testing.T) {
	o := NewOverlay()
	o.OpenModels("deepseek-v4-flash")

	if got := len(o.VisibleRows()); got != len(availableModels()) {
		t.Fatalf("empty filter should show all %d models, got %d", len(availableModels()), got)
	}

	// "pro" should narrow to the single pro row.
	for _, r := range "pro" {
		o.FilterType(r)
	}
	vis := o.VisibleRows()
	if len(vis) == 0 {
		t.Fatal("filter \"pro\" matched no rows")
	}
	// The top-ranked row under the cursor must be the pro model.
	if id := o.SelectedModelID(); id != "deepseek-v4-pro" {
		t.Fatalf("filter \"pro\" should select deepseek-v4-pro, got %q", id)
	}
	if len(vis) >= len(availableModels()) {
		t.Fatalf("filter \"pro\" should narrow the set, still showing %d/%d", len(vis), len(availableModels()))
	}

	// Backspacing the whole filter restores the full list.
	o.FilterBackspace()
	o.FilterBackspace()
	o.FilterBackspace()
	if got := len(o.VisibleRows()); got != len(availableModels()) {
		t.Fatalf("clearing the filter should restore all %d models, got %d", len(availableModels()), got)
	}
}

// TestPickerEscClearsThenCloses: the first esc on a filterable overlay clears a
// non-empty filter; only a second esc closes the overlay.
func TestPickerEscClearsThenCloses(t *testing.T) {
	a := newKeyflowApp(t)
	a = sizeApp(t, a, 100, 40)
	a.handleSlash("/models")
	if !a.overlay.IsOpen() {
		t.Fatal("/models should open the picker")
	}
	a.overlay.FilterType('x')
	if a.overlay.FilterString() != "x" {
		t.Fatalf("filter should be \"x\", got %q", a.overlay.FilterString())
	}

	// First esc: clears the filter, overlay stays open.
	a.handleOverlayKey(keyEscape())
	if !a.overlay.IsOpen() {
		t.Fatal("first esc with a non-empty filter must clear it, not close the overlay")
	}
	if a.overlay.FilterString() != "" {
		t.Fatalf("first esc should clear the filter, got %q", a.overlay.FilterString())
	}

	// Second esc: closes the overlay.
	a.handleOverlayKey(keyEscape())
	if a.overlay.IsOpen() {
		t.Fatal("second esc on an empty filter must close the overlay")
	}
}

// --- G5: command palette ---------------------------------------------------

// TestPaletteOpensFromInsertAndNormal: ctrl+p opens the palette in both Insert
// and Normal mode (the only two interactive modes), without colliding with the
// reasoning folds.
func TestPaletteOpensFromInsertAndNormal(t *testing.T) {
	for _, mode := range []appMode{modeInsert, modeNormal} {
		a := newKeyflowApp(t)
		a = sizeApp(t, a, 100, 40)
		a.setMode(mode)
		a.Update(ctrl('p'))
		if a.overlay.Mode() != modePalette {
			t.Fatalf("ctrl+p from mode %v should open the palette, got mode %v", mode, a.overlay.Mode())
		}
	}
}

// TestPaletteListsCommandsAndExtraVerbs: the palette is a superset of the /
// menu — every merged slash command plus the non-slash verbs.
func TestPaletteListsCommandsAndExtraVerbs(t *testing.T) {
	a := newKeyflowApp(t)
	a = sizeApp(t, a, 100, 40)
	acts := a.paletteActions()

	ids := map[string]bool{}
	for _, x := range acts {
		ids[x.id] = true
	}
	for _, want := range []string{"/help", "/models", "/clear", "/undo", "/compact"} {
		if !ids[want] {
			t.Errorf("palette is missing slash action %q", want)
		}
	}
	for _, want := range []string{"toggle-thinking", "open-help", "yank-last"} {
		if !ids[want] {
			t.Errorf("palette is missing the non-slash verb %q", want)
		}
	}
}

// TestPaletteRunsToggleThinking: selecting the "toggle thinking" verb flips the
// agent's per-turn reasoning flag (a non-slash verb exercised end to end).
func TestPaletteRunsToggleThinking(t *testing.T) {
	a := newKeyflowApp(t)
	a = sizeApp(t, a, 100, 40)
	before := a.thinking

	a.openPalette()
	// Filter to the toggle-thinking row and run it with enter.
	for _, r := range "toggle thinking" {
		a.overlay.FilterType(r)
	}
	act, ok := a.overlay.SelectedAction()
	if !ok || act.id != "toggle-thinking" {
		t.Fatalf("filter \"toggle thinking\" should select the toggle-thinking verb, got %q (ok=%v)", act.id, ok)
	}
	a.handleOverlayKey(keyEnter())

	if a.overlay.IsOpen() {
		t.Fatal("running a palette action should close the palette")
	}
	if a.thinking == before {
		t.Fatalf("the toggle-thinking verb should flip a.thinking (was %v, still %v)", before, a.thinking)
	}
	if a.agent != nil && a.agent.Thinking != a.thinking {
		t.Fatalf("toggle-thinking should keep agent.Thinking in lockstep: app=%v agent=%v", a.thinking, a.agent.Thinking)
	}
}

// TestPaletteRunsSlashAction: selecting a slash-backed row routes through
// handleSlash. We pick /clear because its effect (empty scrollback) is
// observable without a network call.
func TestPaletteRunsSlashAction(t *testing.T) {
	a := newKeyflowApp(t)
	a = sizeApp(t, a, 100, 40)
	a.scrollback.AppendInfo("a line to clear")
	a.refreshView()
	if len(a.scrollback.Items()) == 0 {
		t.Fatal("precondition: scrollback should have content")
	}

	a.openPalette()
	for _, r := range "clear" {
		a.overlay.FilterType(r)
	}
	act, ok := a.overlay.SelectedAction()
	if !ok || act.id != "/clear" {
		t.Fatalf("filter \"clear\" should select /clear, got %q (ok=%v)", act.id, ok)
	}
	a.handleOverlayKey(keyEnter())

	if len(a.scrollback.Items()) != 0 {
		t.Fatalf("/clear via palette should empty the scrollback, %d items remain", len(a.scrollback.Items()))
	}
}

// --- G7: help overlay ------------------------------------------------------

// TestHelpOpensAndCloses: ctrl+g opens the help overlay from any mode, and
// esc/q closes it.
func TestHelpOpensAndCloses(t *testing.T) {
	a := newKeyflowApp(t)
	a = sizeApp(t, a, 100, 40)

	a.Update(ctrl('g'))
	if a.overlay.Mode() != modeHelp {
		t.Fatalf("ctrl+g should open help, got mode %v", a.overlay.Mode())
	}
	// q closes (modeHelp is not filterable, so q is a close key, not filter).
	a.handleOverlayKey(press('q'))
	if a.overlay.IsOpen() {
		t.Fatal("q should close the help overlay")
	}

	a.Update(ctrl('g'))
	a.handleOverlayKey(keyEscape())
	if a.overlay.IsOpen() {
		t.Fatal("esc should close the help overlay")
	}
}

// TestHelpQuestionMarkNormalMode: `?` opens help in Normal mode.
func TestHelpQuestionMarkNormalMode(t *testing.T) {
	a := newKeyflowApp(t)
	a = sizeApp(t, a, 100, 40)
	a.setMode(modeNormal)

	a = drive(t, a, press('?'))
	if a.overlay.Mode() != modeHelp {
		t.Fatalf("? in Normal mode should open help, got mode %v", a.overlay.Mode())
	}
}

// TestHelpQuestionMarkLiteralInInsert: `?` stays a literal character while
// typing — it must flow to the textarea and NOT open help.
func TestHelpQuestionMarkLiteralInInsert(t *testing.T) {
	a := newKeyflowApp(t)
	a = sizeApp(t, a, 100, 40)
	a.setMode(modeInsert)

	a = drive(t, a, press('?'))
	if a.overlay.IsOpen() {
		t.Fatal("? in Insert mode must NOT open help — it is a literal character")
	}
	if got := a.input.Value(); got != "?" {
		t.Fatalf("? in Insert mode should type a literal '?', input = %q", got)
	}
}

// TestHelpBodyGeneratedFromRegistry: the help body lists the built-in commands
// from the shared registry (so it can't drift from the / menu) and the static
// keybinding table.
func TestHelpBodyGeneratedFromRegistry(t *testing.T) {
	rows := allCommands(nil, nil)
	body := helpBody(rows)
	for _, want := range []string{"/help", "/models", "/compact"} {
		if !strings.Contains(body, want) {
			t.Errorf("help body should list %q from the registry", want)
		}
	}
	// The keybinding table half must be present too.
	for _, want := range []string{"^P", "^G", "^R"} {
		if !strings.Contains(body, want) {
			t.Errorf("help body should document the %q keybinding", want)
		}
	}
}

// TestHelpScrollsWithJK: j/k scroll the help overlay (it is not filterable, so
// j/k are navigation, not filter input) and the offset clamps at the top.
func TestHelpScrollsWithJK(t *testing.T) {
	a := newKeyflowApp(t)
	a = sizeApp(t, a, 100, 12) // a short window so the body overflows
	a.openHelp()
	if a.overlay.Cursor() != 0 {
		t.Fatalf("help should open scrolled to the top, offset = %d", a.overlay.Cursor())
	}
	a.handleOverlayKey(press('j'))
	if a.overlay.Cursor() != 1 {
		t.Fatalf("j should scroll the help down by one, offset = %d", a.overlay.Cursor())
	}
	a.handleOverlayKey(press('k'))
	a.handleOverlayKey(press('k')) // clamp at the top
	if a.overlay.Cursor() != 0 {
		t.Fatalf("k should clamp the help scroll at the top, offset = %d", a.overlay.Cursor())
	}
	// Rendering at the short height must not panic and must produce a pane.
	if got := renderHelp(a.theme, a.helpCommandRows(), a.overlay.Cursor(), 100, 10); got == "" {
		t.Fatal("renderHelp should produce a non-empty pane")
	}
}

// TestHelpSlashOpensOverlay: /help opens the overlay rather than dumping text
// into the scrollback (the G7 replacement of the old scrollback dump).
func TestHelpSlashOpensOverlay(t *testing.T) {
	a := newKeyflowApp(t)
	a = sizeApp(t, a, 100, 40)
	before := len(a.scrollback.Items())

	a.handleSlash("/help")
	if a.overlay.Mode() != modeHelp {
		t.Fatalf("/help should open the help overlay, got mode %v", a.overlay.Mode())
	}
	if got := len(a.scrollback.Items()); got != before {
		t.Fatalf("/help must not dump into the scrollback (grew by %d)", got-before)
	}
}
