package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/amemiya02/deepseekcode/internal/agent"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// Key flow is named in CLAUDE.md as the single most fragile TUI
// invariant: Update() must return early ONLY when handleKey reports
// intercepted=true, and the textarea must receive a keystroke ONLY when
// handleKey lets it fall through. These tests pin that contract so a
// regression — a plain-letter binding, or j/k leaking into the input
// after /clear — fails here instead of in a user's terminal.
//
// The observable proxy for "the keystroke reached the textarea" is
// a.input.Value(): the textarea only mutates its value when Update
// forwards the msg to a.input.Update, which happens exclusively in
// modeInsert and exclusively when handleKey did not intercept.

// newKeyflowApp builds a minimally-wired App suitable for driving
// Update/handleKey/setMode without any network or agent Run. The agent
// is constructed with nil llm.Client / empty registry / permissive
// policy; none of the key-flow paths exercised here call Agent.Run, so
// no API key or transport is needed. It is laid out at a fixed size so
// the viewport/textarea geometry is initialised (layout() runs on the
// WindowSizeMsg the tests send first).
func newKeyflowApp(t *testing.T) *App {
	t.Helper()
	reg := tools.New()
	pol := permissions.New(permissions.ModeDefault, t.TempDir(), nil, nil, nil)
	ag := agent.New(nil, reg, pol, "deepseek-v4-flash")
	app := New(Config{
		Agent: ag,
		Model: "deepseek-v4-flash",
		Cwd:   t.TempDir(),
	})
	// Give the program a no-op send so any code path that calls a.send
	// during a test does not nil-panic. The key-flow paths here don't,
	// but this keeps the harness robust.
	app.send = func(tea.Msg) {}
	return app
}

// sizeApp delivers an initial WindowSizeMsg so layout() runs and the
// viewport/textarea have real dimensions. Returns the model as *App.
func sizeApp(t *testing.T, a *App, w, h int) *App {
	t.Helper()
	m, _ := a.Update(tea.WindowSizeMsg{Width: w, Height: h})
	got, ok := m.(*App)
	if !ok {
		t.Fatalf("Update returned %T, want *App", m)
	}
	return got
}

// press builds a synthetic printable-character key press. text must be a
// single rune that the textarea would insert verbatim.
func press(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

// ctrl builds a synthetic ctrl+<r> key press (e.g. ctrl('r') → "ctrl+r").
func ctrl(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl}
}

func keyEnter() tea.KeyPressMsg  { return tea.KeyPressMsg{Code: tea.KeyEnter} }
func keyEscape() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyEscape} }
func shiftEnter() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}
}

// drive sends a key through Update and returns the resulting *App.
func drive(t *testing.T, a *App, km tea.KeyPressMsg) *App {
	t.Helper()
	m, _ := a.Update(km)
	got, ok := m.(*App)
	if !ok {
		t.Fatalf("Update returned %T, want *App", m)
	}
	return got
}

// --- The intercepted contract --------------------------------------------

// TestHandleKeyInterceptStringConsistency verifies the synthetic key
// strings the rest of the suite relies on actually render the way the
// production switch statements expect. If bubbletea changes how it
// stringifies these, the whole harness would silently test nothing —
// this guards against that.
func TestHandleKeyInterceptStringConsistency(t *testing.T) {
	cases := []struct {
		km   tea.KeyPressMsg
		want string
	}{
		{press('j'), "j"},
		{press('k'), "k"},
		{press('i'), "i"},
		{ctrl('r'), "ctrl+r"},
		{ctrl('t'), "ctrl+t"},
		{ctrl('c'), "ctrl+c"},
		{keyEnter(), "enter"},
		{keyEscape(), "esc"},
	}
	for _, c := range cases {
		if got := c.km.String(); got != c.want {
			t.Errorf("KeyPressMsg.String() = %q, want %q", got, c.want)
		}
	}
}

// TestInsertModeFallThroughReachesTextarea: in Insert mode, a plain
// printable key is NOT intercepted by handleKey, so Update must fall
// through and forward it to a.input.Update — the character lands in the
// textarea value.
func TestInsertModeFallThroughReachesTextarea(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	if a.mode != modeInsert {
		t.Fatalf("expected default mode modeInsert, got %v", a.mode)
	}
	if _, intercepted := a.handleKey(press('j')); intercepted {
		t.Fatalf("handleKey intercepted plain 'j' in Insert mode; it must fall through to the textarea")
	}
	a = drive(t, a, press('j'))
	if got := a.input.Value(); got != "j" {
		t.Fatalf("after typing 'j' in Insert mode, input value = %q, want %q", got, "j")
	}
}

// TestShiftEnterInsertsNewline: Shift+Enter must insert a newline into the
// textarea (not submit).  handleKey reports intercepted=false (the hasShiftEnter
// fast-path), and the textarea's InsertNewline binding includes "shift+enter"
// so the key splits the current line.
func TestShiftEnterInsertsNewline(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	// Type some text, then Shift+Enter.
	a = drive(t, a, press('h'))
	a = drive(t, a, press('i'))
	// Shift+Enter: handleKey must NOT intercept (falls through to textarea).
	cmd, intercepted := a.handleKey(shiftEnter())
	if intercepted {
		t.Fatalf("handleKey intercepted Shift+Enter; it must fall through to the textarea")
	}
	_ = cmd
	// Drive through Update so the textarea actually processes it.
	a = drive(t, a, shiftEnter())
	got := a.input.Value()
	if got != "hi\n" {
		t.Fatalf("after Shift+Enter, input value = %q, want %q", got, "hi\n")
	}
}

// TestNormalModeJKDoNotReachTextarea: in Normal mode, j/k are bound to
// viewport scroll and handleKey reports intercepted=true, so Update
// returns early and the textarea value never changes. This is the core
// "no plain-letter leakage into navigation" invariant.
func TestNormalModeJKDoNotReachTextarea(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	// Switch to Normal mode the way the app does (esc from Insert).
	a = drive(t, a, keyEscape())
	if a.mode != modeNormal {
		t.Fatalf("after esc, expected modeNormal, got %v", a.mode)
	}
	for _, r := range []rune{'j', 'k'} {
		cmd, intercepted := a.handleKey(press(r))
		if !intercepted {
			t.Fatalf("handleKey did NOT intercept %q in Normal mode; it would leak into the textarea", string(r))
		}
		_ = cmd
	}
	// Drive them through Update and confirm nothing landed in the input.
	a = drive(t, a, press('j'))
	a = drive(t, a, press('k'))
	if got := a.input.Value(); got != "" {
		t.Fatalf("j/k leaked into textarea in Normal mode: input value = %q, want empty", got)
	}
}

// TestNormalModePrintableFallsThroughAndAutoInserts: a printable key
// that is NOT a Normal-mode binding (e.g. 'z') must NOT be intercepted;
// Update auto-switches to Insert and the same keystroke flows through to
// the textarea on the same cycle (per the comment in Update).
func TestNormalModePrintableFallsThroughAndAutoInserts(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	a = drive(t, a, keyEscape())
	if a.mode != modeNormal {
		t.Fatalf("expected modeNormal, got %v", a.mode)
	}
	if _, intercepted := a.handleKey(press('z')); intercepted {
		t.Fatalf("handleKey intercepted non-binding printable 'z' in Normal mode; it should fall through")
	}
	a = drive(t, a, press('z'))
	if a.mode != modeInsert {
		t.Fatalf("printable key in Normal mode should auto-switch to Insert, got mode %v", a.mode)
	}
	if got := a.input.Value(); got != "z" {
		t.Fatalf("printable key in Normal mode should flow to textarea, input value = %q, want %q", got, "z")
	}
}

// TestNoLeakageAfterClear reproduces the historical bug the Update
// comment warns about: after /clear, typing 'j' must insert 'j', not
// scroll. /clear does not change mode; we stay in Insert. The keystroke
// must fall through and reach the textarea.
func TestNoLeakageAfterClear(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	// Run /clear via the real slash path.
	a.handleSlash("/clear")
	if a.mode != modeInsert {
		t.Fatalf("after /clear, expected to remain in modeInsert, got %v", a.mode)
	}
	if _, intercepted := a.handleKey(press('j')); intercepted {
		t.Fatalf("'j' was intercepted after /clear; it must insert 'j', not scroll")
	}
	a = drive(t, a, press('j'))
	if got := a.input.Value(); got != "j" {
		t.Fatalf("after /clear, typing 'j' should insert 'j' into the input, got %q", got)
	}
}

// TestNoLeakageAfterClearInNormalMode: if the user was in Normal mode
// when they cleared, j/k must STILL be navigation (intercepted) and must
// not leak into the input — the clear path must not silently flip the
// routing so plain letters become text in a blurred input.
func TestNoLeakageAfterClearInNormalMode(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	a = drive(t, a, keyEscape()) // → Normal
	a.handleSlash("/clear")
	if a.mode != modeNormal {
		t.Fatalf("/clear should not change mode; expected modeNormal, got %v", a.mode)
	}
	if _, intercepted := a.handleKey(press('j')); !intercepted {
		t.Fatalf("after /clear in Normal mode, 'j' must remain intercepted (scroll), not leak as text")
	}
	a = drive(t, a, press('j'))
	if got := a.input.Value(); got != "" {
		t.Fatalf("after /clear in Normal mode, 'j' leaked into the input: %q", got)
	}
}

// --- Reasoning fold is ctrl+r / ctrl+t, never plain letters ---------------

// TestReasoningFoldBoundToCtrlKeys: ctrl+r and ctrl+t toggle the
// reasoning fold in BOTH Insert and Normal modes, are intercepted, and
// never reach the textarea. The plain letters 'r' and 't' must NOT
// toggle anything — in Insert mode they are ordinary text.
func TestReasoningFoldBoundToCtrlKeys(t *testing.T) {
	// ctrl+r in Insert mode: intercepted, input untouched.
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	if _, intercepted := a.handleKey(ctrl('r')); !intercepted {
		t.Fatalf("ctrl+r must be intercepted in Insert mode (reasoning fold)")
	}
	a = drive(t, a, ctrl('r'))
	if got := a.input.Value(); got != "" {
		t.Fatalf("ctrl+r leaked into the textarea: %q", got)
	}

	// ctrl+t in Insert mode: intercepted, input untouched.
	a = sizeApp(t, newKeyflowApp(t), 100, 40)
	if _, intercepted := a.handleKey(ctrl('t')); !intercepted {
		t.Fatalf("ctrl+t must be intercepted in Insert mode (reasoning fold)")
	}
	a = drive(t, a, ctrl('t'))
	if got := a.input.Value(); got != "" {
		t.Fatalf("ctrl+t leaked into the textarea: %q", got)
	}

	// ctrl+r / ctrl+t in Normal mode: intercepted.
	a = sizeApp(t, newKeyflowApp(t), 100, 40)
	a = drive(t, a, keyEscape())
	if _, intercepted := a.handleKey(ctrl('r')); !intercepted {
		t.Fatalf("ctrl+r must be intercepted in Normal mode (reasoning fold)")
	}
	if _, intercepted := a.handleKey(ctrl('t')); !intercepted {
		t.Fatalf("ctrl+t must be intercepted in Normal mode (reasoning fold)")
	}
}

// TestPlainLetterIsNotReasoningFold: plain 'r' and 't' must be ordinary
// printable text in Insert mode (fall through to textarea), NOT a
// reasoning-fold shortcut. This is the inverse guard: it fails if anyone
// rebinds the fold onto a plain letter (which the CLAUDE.md invariant
// forbids — plain letters collide with first-character typing).
func TestPlainLetterIsNotReasoningFold(t *testing.T) {
	for _, r := range []rune{'r', 't'} {
		a := sizeApp(t, newKeyflowApp(t), 100, 40)
		if _, intercepted := a.handleKey(press(r)); intercepted {
			t.Fatalf("plain %q was intercepted in Insert mode; reasoning fold must be ctrl+%[1]s, not a plain letter", string(r))
		}
		a = drive(t, a, press(r))
		if got := a.input.Value(); got != string(r) {
			t.Fatalf("plain %q should be inserted as text in Insert mode, got input value %q", string(r), got)
		}
	}
}

// --- Update returns early iff handleKey intercepts -------------------------

// TestUpdateForwardsOnlyInInsertMode confirms the structural guarantee:
// a blurred mode (Normal) never forwards a printable binding key to the
// textarea, while Insert mode forwards a non-intercepted key. We assert
// this against the input value across a representative key in each mode.
func TestUpdateForwardsOnlyInInsertMode(t *testing.T) {
	// Insert mode, non-intercepted key 'a' → forwarded.
	insert := sizeApp(t, newKeyflowApp(t), 100, 40)
	insert = drive(t, insert, press('a'))
	if got := insert.input.Value(); got != "a" {
		t.Fatalf("Insert mode should forward 'a' to textarea, got %q", got)
	}

	// Normal mode, intercepted binding 'j' → not forwarded.
	normal := sizeApp(t, newKeyflowApp(t), 100, 40)
	normal = drive(t, normal, keyEscape())
	normal = drive(t, normal, press('j'))
	if got := normal.input.Value(); got != "" {
		t.Fatalf("Normal mode should not forward 'j' to textarea, got %q", got)
	}
}

// TestEscFromInsertIsInterceptedAndSwitchesMode: esc is intercepted in
// Insert mode and switches to Normal; it must not leak into the input.
func TestEscFromInsertIsInterceptedAndSwitchesMode(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	if _, intercepted := a.handleKey(keyEscape()); !intercepted {
		t.Fatalf("esc must be intercepted in Insert mode")
	}
	a = drive(t, a, keyEscape())
	if a.mode != modeNormal {
		t.Fatalf("esc in Insert mode should switch to Normal, got %v", a.mode)
	}
	if got := a.input.Value(); got != "" {
		t.Fatalf("esc leaked into textarea: %q", got)
	}
}

// --- Agent-event dispatch does not perturb the key-flow routing -----------

// TestAgentEventDispatchKeepsInputRouting: dispatching an agent event
// (e.g. an info line) through Update must not change the mode or leak
// into the textarea, and a subsequent printable key in Insert mode must
// still reach the input. This pins that the agent-event lane and the
// key-flow lane stay independent.
func TestAgentEventDispatchKeepsInputRouting(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	m, _ := a.Update(agentEventMsg{Event: agent.EventInfo{Text: "hello from the agent"}})
	a, ok := m.(*App)
	if !ok {
		t.Fatalf("Update returned %T, want *App", m)
	}
	if a.mode != modeInsert {
		t.Fatalf("agent event should not change mode; got %v", a.mode)
	}
	if got := a.input.Value(); got != "" {
		t.Fatalf("agent event leaked into textarea: %q", got)
	}
	// Typing still works after an agent event.
	a = drive(t, a, press('x'))
	if got := a.input.Value(); got != "x" {
		t.Fatalf("after agent event, typing 'x' should reach textarea, got %q", got)
	}
}

// TestPermissionModeEatsKeys: while a permission ask is up, the modal
// owns input and the textarea must not see keys, even unmapped ones.
// Update routes KeyPressMsg to handlePermissionKey and returns before the
// fall-through forward block.
func TestPermissionModeEatsKeys(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	// Drive an EventPermissionAsk through dispatch to open the modal the
	// real way (sets mode to modePermission via setMode).
	a.dispatchAgentEvent(agent.EventPermissionAsk{
		Check: permissions.Check{},
		Reply: make(chan agent.PermissionResponse, 1),
	})
	if a.mode != modePermission {
		t.Fatalf("EventPermissionAsk should put app in modePermission, got %v", a.mode)
	}
	// An unmapped printable key must be swallowed, not inserted.
	a = drive(t, a, press('z'))
	if got := a.input.Value(); got != "" {
		t.Fatalf("permission modal leaked 'z' into the textarea: %q", got)
	}
	if a.mode != modePermission {
		t.Fatalf("permission modal should stay up after an unmapped key, got mode %v", a.mode)
	}
}

// --- G1/G2/G3: inline completions popup + prompt history wiring -----------
//
// These pin the new Insert-mode precedence chain: the `/` menu is derived
// from the post-keystroke buffer,
// ↑/↓ recall history only at the cursor's top/bottom edge, accepting a
// completion inserts without submitting, and a real Enter still submits.

func keyUp() tea.KeyPressMsg   { return tea.KeyPressMsg{Code: tea.KeyUp} }
func keyDown() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyDown} }
func keyTab() tea.KeyPressMsg  { return tea.KeyPressMsg{Code: tea.KeyTab} }

// TestSlashOpensPopupViaBufferSync: typing '/' is an ordinary printable key
// that falls through to the textarea; syncCompletions then derives the popup
// from the buffer and opens the `/` menu. The popup must reserve layout rows
// so it cannot push the input off-screen.
func TestSlashOpensPopupViaBufferSync(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	if a.completions.Active() {
		t.Fatal("popup should start closed")
	}
	a = drive(t, a, press('/'))
	if got := a.input.Value(); got != "/" {
		t.Fatalf("typing '/' should reach the textarea, input = %q", got)
	}
	if !a.completions.Active() {
		t.Fatal("typing '/' at a word boundary should open the / menu")
	}
	if a.completions.Trigger() != '/' {
		t.Fatalf("popup trigger = %q, want '/'", a.completions.Trigger())
	}
	if a.popupLines <= 0 {
		t.Fatalf("popupLines should be reserved while the menu is open, got %d", a.popupLines)
	}
	// Filtering narrows as the query grows: "/mod" should still match /models.
	a = drive(t, a, press('m'))
	a = drive(t, a, press('o'))
	a = drive(t, a, press('d'))
	sel, ok := a.completions.Selected()
	if !ok {
		t.Fatal("expected a selected completion after typing /mod")
	}
	if sel.insert != "/models" {
		t.Fatalf("/mod should select /models, got %q", sel.insert)
	}
}

// TestSlashMidWordIsInert: a '/' that is not at a word boundary (e.g. inside
// a path token) must NOT open the menu — only a token-leading trigger does.
func TestSlashMidWordIsInert(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	a = drive(t, a, press('a'))
	a = drive(t, a, press('/'))
	if got := a.input.Value(); got != "a/" {
		t.Fatalf("input = %q, want %q", got, "a/")
	}
	if a.completions.Active() {
		t.Fatal("a mid-word '/' must not open the menu")
	}
	if a.popupLines != 0 {
		t.Fatalf("popupLines should be 0 with no menu, got %d", a.popupLines)
	}
}

// TestAcceptCompletionInsertsAndDoesNotSubmit: with the popup open, Enter
// accepts the selected command into the input and closes the popup; it must
// NOT submit (no user line appended, input retains the command text). A
// second Enter is what actually submits.
func TestAcceptCompletionInsertsAndDoesNotSubmit(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	a = drive(t, a, press('/'))
	if !a.completions.Active() {
		t.Fatal("expected the / menu open")
	}
	sel, _ := a.completions.Selected()
	want := sel.insert

	beforeItems := len(a.scrollback.Items())
	a = drive(t, a, keyEnter())

	if a.completions.Active() {
		t.Fatal("accepting a completion should close the popup")
	}
	if got := a.input.Value(); got != want {
		t.Fatalf("accept should insert %q into the input, got %q", want, got)
	}
	if len(a.scrollback.Items()) != beforeItems {
		t.Fatal("accepting a completion must NOT submit (no scrollback item should be added)")
	}
	if a.popupLines != 0 {
		t.Fatalf("popupLines should reset to 0 after accept, got %d", a.popupLines)
	}
}

// TestTabAlsoAcceptsCompletion: Tab is an accept alias while the menu is open.
func TestTabAlsoAcceptsCompletion(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	a = drive(t, a, press('/'))
	sel, _ := a.completions.Selected()
	want := sel.insert
	a = drive(t, a, keyTab())
	if a.completions.Active() {
		t.Fatal("Tab should accept and close the popup")
	}
	if got := a.input.Value(); got != want {
		t.Fatalf("Tab accept should insert %q, got %q", want, got)
	}
}

// TestEscClosesPopupStaysInsert: Esc with the popup open closes the menu only
// and stays in Insert mode (it does not fall through to the Normal-mode
// switch). Esc with no popup still leaves to Normal.
func TestEscClosesPopupStaysInsert(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	a = drive(t, a, press('/'))
	if !a.completions.Active() {
		t.Fatal("expected the / menu open")
	}
	a = drive(t, a, keyEscape())
	if a.completions.Active() {
		t.Fatal("Esc should close the popup")
	}
	if a.mode != modeInsert {
		t.Fatalf("Esc with a popup open should stay in Insert, got mode %v", a.mode)
	}
	// With no popup, Esc now leaves to Normal.
	a = drive(t, a, keyEscape())
	if a.mode != modeNormal {
		t.Fatalf("Esc with no popup should switch to Normal, got %v", a.mode)
	}
}

// TestEnterSubmitsNormalPrompt: a plain prompt (no popup, no slash) still
// submits — the user line is appended and the input is cleared.
func TestEnterSubmitsNormalPrompt(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	a.input.SetValue("hello world")
	if a.completions.Active() {
		t.Fatal("a plain prompt should not have a popup open")
	}
	before := len(a.scrollback.Items())
	a = drive(t, a, keyEnter())
	if got := a.input.Value(); got != "" {
		t.Fatalf("submit should clear the input, got %q", got)
	}
	if len(a.scrollback.Items()) <= before {
		t.Fatal("submitting a normal prompt should append a user line to the scrollback")
	}
	// The submitted prompt joins the recall ring.
	ents := a.history.Entries()
	if len(ents) == 0 || ents[len(ents)-1] != "hello world" {
		t.Fatalf("submit should add the prompt to history, entries = %v", ents)
	}
}

// TestHistoryRecallAtTopEdge: with a single-line draft the cursor is on row 0,
// so ↑ recalls the previous prompt and ↓ walks back to the saved draft.
func TestHistoryRecallAtTopEdge(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	a.history = newPromptHistory([]string{"older", "newer"}, 500)

	a.input.SetValue("draft")
	a = drive(t, a, keyUp())
	if got := a.input.Value(); got != "newer" {
		t.Fatalf("↑ on row 0 should recall newest entry 'newer', got %q", got)
	}
	a = drive(t, a, keyUp())
	if got := a.input.Value(); got != "older" {
		t.Fatalf("second ↑ should recall 'older', got %q", got)
	}
	a = drive(t, a, keyDown())
	if got := a.input.Value(); got != "newer" {
		t.Fatalf("↓ should step back to 'newer', got %q", got)
	}
	a = drive(t, a, keyDown())
	if got := a.input.Value(); got != "draft" {
		t.Fatalf("↓ past the newest entry should restore the live draft, got %q", got)
	}
}

// TestHistoryEdgeGatingMidDraft: ↑ on a NON-top row of a multi-line draft must
// move the textarea cursor (multiline editing) and must NOT recall history.
// This is the readline edge rule from §5.2.
func TestHistoryEdgeGatingMidDraft(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	a.history = newPromptHistory([]string{"prev-prompt"}, 500)

	// A 3-line draft; SetValue parks the cursor at the end (row 2).
	a.input.SetValue("line1\nline2\nline3")
	if a.input.Line() != a.input.LineCount()-1 {
		t.Fatalf("expected cursor on the last row after SetValue, got row %d of %d", a.input.Line(), a.input.LineCount())
	}

	// ↑ from the last row: cursor moves up to row 1, draft is untouched, and
	// history is NOT consulted (idx stays -1, value unchanged).
	a = drive(t, a, keyUp())
	if got := a.input.Value(); got != "line1\nline2\nline3" {
		t.Fatalf("↑ on a non-top row must not recall history; value = %q", got)
	}
	if a.input.Line() == a.input.LineCount()-1 {
		t.Fatalf("↑ should have moved the cursor off the last row, still at row %d", a.input.Line())
	}
	if a.history.idx != -1 {
		t.Fatalf("history must not be browsing after a mid-draft ↑, idx = %d", a.history.idx)
	}
}

// TestAcceptWithQuerySplicesAtAnchor: after narrowing with a query, accepting
// replaces from the trigger anchor to the cursor with the full command,
// leaving a clean "/models" rather than duplicating the typed query. This
// pins the cursorByteOffset + anchor splice (acceptCompletion).
func TestAcceptWithQuerySplicesAtAnchor(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	for _, r := range "/mod" {
		a = drive(t, a, press(r))
	}
	if !a.completions.Active() {
		t.Fatal("expected the / menu open after typing /mod")
	}
	sel, ok := a.completions.Selected()
	if !ok || sel.insert != "/models" {
		t.Fatalf("/mod should select /models, got %+v ok=%v", sel, ok)
	}
	a = drive(t, a, keyEnter())
	if got := a.input.Value(); got != "/models" {
		t.Fatalf("accept after /mod should yield %q, got %q", "/models", got)
	}
	if a.completions.Active() {
		t.Fatal("accept should close the popup")
	}
}

// TestExactSlashCompletionEnterSubmits: once a slash command is typed exactly,
// Enter should execute it rather than requiring a second press to submit the
// already-complete line. Partial queries still accept first (covered above).
func TestExactSlashCompletionEnterSubmits(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	for _, r := range "/theme" {
		a = drive(t, a, press(r))
	}
	if !a.completions.Active() {
		t.Fatal("expected the / menu open after typing /theme")
	}
	sel, ok := a.completions.Selected()
	if !ok || sel.insert != "/theme" {
		t.Fatalf("/theme should select /theme, got %+v ok=%v", sel, ok)
	}

	a = drive(t, a, keyEnter())
	if a.completions.Active() {
		t.Fatal("exact command submit should close the popup")
	}
	if got := a.input.Value(); got != "" {
		t.Fatalf("exact command submit should clear input, got %q", got)
	}
	if a.overlay.Mode() != modeThemes {
		t.Fatalf("expected /theme to open theme picker, got mode %d", a.overlay.Mode())
	}
}

// TestAtTriggerStubDoesNotTrapInput: typing '@' at a word boundary opens the @
// file-mention menu (G4). Before the background walk's fileIndexMsg lands it
// shows the single non-insertable "(indexing files…)" placeholder; Enter on
// that row neither submits nor traps — it just closes, leaving the '@' in the
// buffer.
func TestAtTriggerStubDoesNotTrapInput(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	a = drive(t, a, press('@'))
	if got := a.input.Value(); got != "@" {
		t.Fatalf("typing '@' should reach the textarea, got %q", got)
	}
	if a.completions.Trigger() != '@' || !a.completions.Active() {
		t.Fatalf("'@' should open the @ trigger popup, active=%v trigger=%q", a.completions.Active(), a.completions.Trigger())
	}
	// The placeholder is one row plus the two card borders.
	if a.popupLines != 3 {
		t.Fatalf("the @ indexing placeholder should reserve 3 rows, got %d", a.popupLines)
	}
	// Enter on the placeholder closes it without submitting (empty insert).
	before := len(a.scrollback.Items())
	a = drive(t, a, keyEnter())
	if a.completions.Active() {
		t.Fatal("Enter on the @ placeholder should close the popup")
	}
	if len(a.scrollback.Items()) != before {
		t.Fatal("Enter on the @ placeholder must not submit")
	}
	if got := a.input.Value(); got != "@" {
		t.Fatalf("Enter on the placeholder should leave the '@' intact, got %q", got)
	}
}

// --- theme apply/preview tests (Task 4206) -----------------------------------

func TestApplyThemeSwitchChangesTheme(t *testing.T) {
	a := newKeyflowApp(t)
	a = sizeApp(t, a, 100, 40)
	if a.theme.Name != "dark" {
		t.Fatalf("expected initial theme 'dark', got %q", a.theme.Name)
	}
	a.applyThemeSwitch("aurora")
	if a.theme.Name != "aurora" {
		t.Fatalf("expected theme 'aurora' after switch, got %q", a.theme.Name)
	}
}

func TestApplyThemeSwitchPersists(t *testing.T) {
	var persisted string
	recorder := func(name string) error {
		persisted = name
		return nil
	}
	a := newKeyflowApp(t)
	a.session.setTheme = recorder
	a.applyThemeSwitch("nebula")
	if persisted != "nebula" {
		t.Errorf("expected persist('nebula'), got %q", persisted)
	}

	// With nil setTheme, it should not panic.
	a2 := newKeyflowApp(t)
	a2.session.setTheme = nil
	a2.applyThemeSwitch("aurora") // should not panic
	if a2.theme.Name != "aurora" {
		t.Errorf("expected theme 'aurora' with nil setTheme, got %q", a2.theme.Name)
	}
}

func TestApplyThemeSwitchUnknownRejected(t *testing.T) {
	a := newKeyflowApp(t)
	a = sizeApp(t, a, 100, 40)
	orig := a.theme.Name
	a.applyThemeSwitch("bogus")
	if a.theme.Name != orig {
		t.Errorf("unknown theme should leave theme unchanged: got %q", a.theme.Name)
	}
}

func TestPreviewThemeNoPersist(t *testing.T) {
	var called bool
	recorder := func(name string) error {
		called = true
		return nil
	}
	a := newKeyflowApp(t)
	a.session.setTheme = recorder
	a.previewTheme("midnight")
	if a.theme.Name != "midnight" {
		t.Errorf("expected theme 'midnight' after preview, got %q", a.theme.Name)
	}
	if called {
		t.Error("previewTheme must NOT call setTheme (persist)")
	}
}
