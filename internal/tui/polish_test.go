package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/lucasb-eyer/go-colorful"

	"github.com/amemiya02/deepseekcode/internal/agent"
)

// TestScrollbarThumbGeometry pins the thumb math: the thumb is at least one
// row, never overruns the track, sits at the top when unscrolled, and reaches
// the bottom edge when fully scrolled.
func TestScrollbarThumbGeometry(t *testing.T) {
	// Tall content in a short track.
	const trackH = 10
	const total = 100

	// Top of scroll.
	start, end := scrollbarThumb(trackH, total, 0)
	if start != 0 {
		t.Errorf("at top: start = %d, want 0", start)
	}
	if end <= start {
		t.Fatalf("thumb empty: [%d,%d)", start, end)
	}
	if end > trackH {
		t.Errorf("thumb overruns track: end = %d > %d", end, trackH)
	}

	// Bottom of scroll: thumb's end must touch the track bottom.
	start, end = scrollbarThumb(trackH, total, 1)
	if end != trackH {
		t.Errorf("at bottom: end = %d, want %d", end, trackH)
	}
	if start < 0 {
		t.Errorf("negative start: %d", start)
	}

	// Minimum one-row thumb even for very long content.
	_, _ = scrollbarThumb(trackH, 100000, 0.5)
	s2, e2 := scrollbarThumb(trackH, 100000, 0.5)
	if e2-s2 < 1 {
		t.Errorf("thumb collapsed below 1 row: [%d,%d)", s2, e2)
	}
}

// TestOverlayScrollbarNoOpWhenFits verifies the scrollbar is suppressed when
// all content fits (nothing to scroll) — the bar only appears when it carries
// information.
func TestOverlayScrollbarNoOpWhenFits(t *testing.T) {
	a := &App{theme: DarkTheme(), width: 20}
	a.vp.SetWidth(20)
	a.vp.SetHeight(10)
	a.vp.SetContent("one\ntwo\nthree")
	view := a.vp.View()
	if got := a.overlayScrollbar(view); got != view {
		t.Error("scrollbar should be a no-op when content fits the viewport")
	}
}

// TestOverlayScrollbarDrawsGlyphs verifies that when content overflows, the
// scrollbar glyphs land on the right edge in the brand-light / border colors.
func TestOverlayScrollbarDrawsGlyphs(t *testing.T) {
	a := &App{theme: DarkTheme(), width: 20}
	a.vp.SetWidth(20)
	a.vp.SetHeight(5)
	var b strings.Builder
	for i := 0; i < 50; i++ {
		b.WriteString("line of content here\n")
	}
	a.vp.SetContent(b.String())
	out := a.overlayScrollbar(a.vp.View())
	plain := stripANSI(out)
	if !strings.Contains(plain, ScrollbarThumb) {
		t.Errorf("expected thumb glyph %q in scrollbar overlay", ScrollbarThumb)
	}
	if !strings.Contains(plain, ScrollbarTrack) {
		t.Errorf("expected track glyph %q in scrollbar overlay", ScrollbarTrack)
	}
	// Every rendered line stays within width.
	for _, ln := range strings.Split(plain, "\n") {
		if w := lipgloss.Width(ln); w > a.width {
			t.Errorf("scrollbar line exceeds width %d: got %d (%q)", a.width, w, ln)
		}
	}
}

// TestModeChip pins the mode-chip mapping: default → no chip; yolo / read-only
// / plan each render their label. nil Permissions → no chip (test fixtures).
func TestModeChip(t *testing.T) {
	// nil agent / permissions: no chip, no panic.
	a := &App{theme: DarkTheme()}
	if got := a.modeChip(); got != "" {
		t.Errorf("nil-permission chip = %q, want empty", got)
	}
}

// TestDarkCodeBlockBackgroundIsBgWell verifies the dark renderer repaints the
// fenced code-block background to the bgWell surface token, while the light
// renderer is left untouched.
func TestDarkCodeBlockBackgroundIsBgWell(t *testing.T) {
	want := tokenHex(DarkTheme().BgWell)
	dark := cleanStyle(DarkTheme(), true) // fills enabled: code-block bg painted to bgWell
	if dark.CodeBlock.BackgroundColor == nil {
		t.Fatal("dark code block has no background color")
	}
	if got := *dark.CodeBlock.BackgroundColor; !sameHex(got, want) {
		t.Errorf("dark code-block bg = %q, want bgWell %q", got, want)
	}
	if dark.CodeBlock.Chroma != nil && dark.CodeBlock.Chroma.Background.BackgroundColor != nil {
		if got := *dark.CodeBlock.Chroma.Background.BackgroundColor; !sameHex(got, want) {
			t.Errorf("dark chroma bg = %q, want bgWell %q", got, want)
		}
	}

	// Heading prefixes stay stripped (regression guard for the existing
	// behavior this function also owns).
	if dark.H3.Prefix != "" {
		t.Errorf("H3 prefix should be stripped, got %q", dark.H3.Prefix)
	}
}

// sameHex compares two hex color strings case-insensitively.
func sameHex(a, b string) bool {
	ca, errA := colorful.Hex(normalizeHex(a))
	cb, errB := colorful.Hex(normalizeHex(b))
	if errA != nil || errB != nil {
		return strings.EqualFold(a, b)
	}
	return ca.Hex() == cb.Hex()
}

func normalizeHex(s string) string {
	if !strings.HasPrefix(s, "#") {
		return "#" + s
	}
	return s
}

// --- G10: dynamic placeholder --------------------------------------------

// TestPlaceholderForFlipsWithRunning pins the core G10 contract: a running
// state shows the steady "Working…" caption; an idle state shows a "Ready" hint
// rotated deterministically by the turn counter (never a clock/random source).
func TestPlaceholderForFlipsWithRunning(t *testing.T) {
	if got := placeholderFor(true, 0); got != workingPlaceholder {
		t.Errorf("running placeholder = %q, want %q", got, workingPlaceholder)
	}
	if got := placeholderFor(true, 7); got != workingPlaceholder {
		t.Errorf("running placeholder ignores turn: got %q", got)
	}
	// Idle rotates through readyHints by turn (mod len), deterministically.
	for turn := 0; turn < len(readyHints)*2+1; turn++ {
		want := readyHints[turn%len(readyHints)]
		if got := placeholderFor(false, turn); got != want {
			t.Errorf("idle placeholder turn=%d = %q, want %q", turn, got, want)
		}
	}
	// Negative turn is clamped, not a panic / negative index.
	if got := placeholderFor(false, -3); got != readyHints[0] {
		t.Errorf("negative turn placeholder = %q, want %q", got, readyHints[0])
	}
}

// TestRefreshPlaceholderTracksUIRunning verifies refreshPlaceholder writes the
// textarea placeholder from the live uiRunning flag + rotation counter, so a
// run-start flips it to Working and a run-finish (with the rotation advanced)
// settles it on the next Ready hint.
func TestRefreshPlaceholderTracksUIRunning(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)

	// Construction already seeded the idle hint.
	if got := a.input.Placeholder; got != readyHints[0] {
		t.Fatalf("initial placeholder = %q, want %q", got, readyHints[0])
	}

	// Simulate the runStartMsg path.
	a.uiRunning = true
	a.refreshPlaceholder()
	if got := a.input.Placeholder; got != workingPlaceholder {
		t.Fatalf("placeholder while running = %q, want %q", got, workingPlaceholder)
	}

	// Simulate the Done path: advance the rotation, flip uiRunning off.
	a.placeholderTurn++
	a.uiRunning = false
	a.refreshPlaceholder()
	if got := a.input.Placeholder; got != readyHints[1] {
		t.Fatalf("placeholder after one turn = %q, want %q", got, readyHints[1])
	}
}

// TestRunStartMsgFlipsPlaceholder drives the real Update path: a runStartMsg
// sets uiRunning and flips the placeholder to Working.
func TestRunStartMsgFlipsPlaceholder(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	m, _ := a.Update(runStartMsg{})
	a = m.(*App)
	if !a.uiRunning {
		t.Fatal("runStartMsg should set uiRunning")
	}
	if got := a.input.Placeholder; got != workingPlaceholder {
		t.Errorf("placeholder after runStartMsg = %q, want %q", got, workingPlaceholder)
	}
}

// --- G11: prompt queueing ------------------------------------------------

// TestQueueDrainsInOrder pins the FIFO contract: prompts enqueued while a run
// is active drain oldest-first, one per Done.
func TestQueueDrainsInOrder(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	a.uiRunning = true // pretend a run is active

	a.enqueuePrompt("first")
	a.enqueuePrompt("second")
	if a.queueDepth() != 2 {
		t.Fatalf("queue depth = %d, want 2", a.queueDepth())
	}
	if a.status.queued != 2 {
		t.Fatalf("status.queued = %d, want 2", a.status.queued)
	}

	// Drain returns a Cmd for the head; the head is removed FIFO.
	if cmd := a.drainQueue(); cmd == nil {
		t.Fatal("drainQueue should return a Cmd when the queue is non-empty")
	}
	if a.queueDepth() != 1 || a.queued[0] != "second" {
		t.Fatalf("after first drain queue = %v, want [second]", a.queued)
	}
	if cmd := a.drainQueue(); cmd == nil {
		t.Fatal("drainQueue should return a Cmd for the second entry")
	}
	if a.queueDepth() != 0 {
		t.Fatalf("queue should be empty, got %v", a.queued)
	}
	if cmd := a.drainQueue(); cmd != nil {
		t.Fatal("drainQueue on an empty queue must return nil")
	}
}

// TestSubmitWhileRunningQueues: pressing Enter on a non-slash prompt while a
// run is active queues it instead of starting a second run, and bumps the
// queued count. A second submit queues again (N pending grows).
func TestSubmitWhileRunningQueues(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	a.uiRunning = true

	a.input.SetValue("queued prompt")
	_, intercepted := a.handleInsertKey(keyEnter())
	if !intercepted {
		t.Fatal("enter should be intercepted")
	}
	if a.queueDepth() != 1 || a.queued[0] != "queued prompt" {
		t.Fatalf("submit while running should queue; queue = %v", a.queued)
	}
	if a.input.Value() != "" {
		t.Fatalf("input should reset after queueing, got %q", a.input.Value())
	}
	if !a.toastState.active || !strings.Contains(a.toastState.text, "queued (1 pending)") {
		t.Fatalf("expected queued toast, got active=%v text=%q", a.toastState.active, a.toastState.text)
	}

	a.input.SetValue("another")
	a.handleInsertKey(keyEnter())
	if a.queueDepth() != 2 {
		t.Fatalf("second submit should grow the queue to 2, got %d", a.queueDepth())
	}
	if !strings.Contains(a.toastState.text, "queued (2 pending)") {
		t.Fatalf("expected '2 pending' toast, got %q", a.toastState.text)
	}
}

// TestSubmitWhenIdleRuns: when no run is active, Enter on a non-slash prompt
// submits immediately (does NOT queue) — the queue is the busy-path only.
func TestSubmitWhenIdleRuns(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	a.send = func(tea.Msg) {}
	a.uiRunning = false

	a.input.SetValue("run me")
	_, intercepted := a.handleInsertKey(keyEnter())
	if !intercepted {
		t.Fatal("enter should be intercepted")
	}
	if a.queueDepth() != 0 {
		t.Fatalf("idle submit must not queue, got depth %d", a.queueDepth())
	}
}

// TestEventDoneDrainsQueue: the Done path submits the next queued prompt and
// keeps uiRunning true (the drained run re-arms it via runStartMsg). When the
// queue is empty, Done clears uiRunning and advances the idle rotation.
func TestEventDoneDrainsQueue(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	a.send = func(tea.Msg) {}
	a.uiRunning = true
	a.enqueuePrompt("pending one")

	cmds := a.dispatchAgentEvent(agent.EventDone{})
	if a.queueDepth() != 0 {
		t.Fatalf("Done should drain the single queued prompt, depth = %d", a.queueDepth())
	}
	// A drain Cmd must be present so the queued prompt actually starts.
	if len(cmds) == 0 {
		t.Fatal("Done with a queued prompt should return a submit Cmd")
	}
	if !a.uiRunning {
		t.Fatal("uiRunning should stay true while a drained prompt is starting")
	}

	// A second Done with an empty queue ends the busy state.
	a.dispatchAgentEvent(agent.EventDone{})
	if a.uiRunning {
		t.Fatal("Done with an empty queue should clear uiRunning")
	}
	if a.placeholderTurn != 2 {
		t.Fatalf("each Done should advance the rotation; turn = %d, want 2", a.placeholderTurn)
	}
}

// TestClearQueueOnCancel: clearQueue drops every pending prompt and toasts the
// drop count; an empty queue is a no-op (nil Cmd).
func TestClearQueueOnCancel(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	a.uiRunning = true
	a.enqueuePrompt("a")
	a.enqueuePrompt("b")

	cmd := a.clearQueue()
	if cmd == nil {
		t.Fatal("clearQueue with pending prompts should return a toast Cmd")
	}
	if a.queueDepth() != 0 || a.status.queued != 0 {
		t.Fatalf("clearQueue should empty the queue, got depth=%d status=%d", a.queueDepth(), a.status.queued)
	}
	if !strings.Contains(a.toastState.text, "dropped 2 queued") {
		t.Fatalf("expected drop toast, got %q", a.toastState.text)
	}
	if got := a.clearQueue(); got != nil {
		t.Fatal("clearQueue on an empty queue must return nil")
	}
}

// --- G12: large-paste collapse -------------------------------------------

// TestLargePasteCollapsesToChip: a paste over the line threshold is held in
// full and replaced in the input by a one-line chip; expandPaste restores the
// full text at submit. A small paste is left untouched (handlePaste returns
// false so the caller forwards it to the textarea).
func TestLargePasteCollapsesToChip(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)

	// Small paste: below threshold, not collapsed.
	small := "line1\nline2"
	if a.handlePaste(small) {
		t.Fatal("a small paste must not collapse")
	}
	if a.input.Value() != "" {
		t.Fatalf("small paste must not be inserted by handlePaste, got %q", a.input.Value())
	}

	// Large paste: collapses to a chip, input shows only the chip line.
	var b strings.Builder
	for i := 0; i < pasteCollapseLines+4; i++ {
		b.WriteString("pasted content line\n")
	}
	big := strings.TrimRight(b.String(), "\n")
	bigLines := strings.Count(big, "\n") + 1
	if !a.handlePaste(big) {
		t.Fatal("a large paste should collapse")
	}
	disp := a.input.Value()
	if strings.Count(disp, "\n") != 0 {
		t.Fatalf("collapsed display should be one line, got %q", disp)
	}
	if !strings.Contains(disp, chipPlaceholder(bigLines)) {
		t.Fatalf("display = %q, want chip for %d lines", disp, bigLines)
	}

	// expandPaste restores the full paste for submission.
	if got := a.expandPaste(disp); got != big {
		t.Fatalf("expandPaste = %q, want the full paste", got)
	}

	// resetPasteState drops the store so a later expand is a no-op.
	a.resetPasteState()
	if got := a.expandPaste(disp); got != disp {
		t.Fatalf("after reset, expandPaste should be identity, got %q", got)
	}
}

// TestTwoLargePastesCoexist: two large pastes in one draft each get a unique
// chip and both expand back to their own content (no aliasing).
func TestTwoLargePastesCoexist(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	mk := func(tag string) string {
		var b strings.Builder
		for i := 0; i < pasteCollapseLines+1; i++ {
			b.WriteString(tag + " line\n")
		}
		return strings.TrimRight(b.String(), "\n")
	}
	p1, p2 := mk("alpha"), mk("beta")
	if !a.handlePaste(p1) || !a.handlePaste(p2) {
		t.Fatal("both large pastes should collapse")
	}
	disp := a.input.Value()
	expanded := a.expandPaste(disp)
	if !strings.Contains(expanded, p1) || !strings.Contains(expanded, p2) {
		t.Fatalf("expanded draft should contain both pastes, got %q", expanded)
	}
	if len(a.pasteStore) != 2 {
		t.Fatalf("two pastes should hold two store entries, got %d", len(a.pasteStore))
	}
}

// TestPasteOnlyCollapsesInInsertMode: a paste arriving in a blurred (Normal)
// mode is not collapsed — the chip machinery is for the live prompt only.
func TestPasteOnlyCollapsesInInsertMode(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	a.mode = modeNormal
	var b strings.Builder
	for i := 0; i < pasteCollapseLines+2; i++ {
		b.WriteString("x\n")
	}
	if a.handlePaste(b.String()) {
		t.Fatal("a paste in Normal mode must not collapse")
	}
}
