package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// G8/G9 cover the transient toast slot and the idle ^C double-tap quit guard.
// These are small state-machine behaviors, so they're table/unit-tested
// directly against handleKey/Update rather than through a rendered frame.

// isQuitCmd runs a tea.Cmd and reports whether it resolves to tea.QuitMsg —
// the signal a second armed ^C (or ctrl+d) returns. A nil cmd is not a quit.
func isQuitCmd(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// TestToastSetThenExpiresViaMsg: toast() arms the slot and returns a tick Cmd;
// delivering the matching clearToastMsg (the tick's payload) clears it. This is
// the full lifecycle without waiting the real TTL.
func TestToastSetThenExpiresViaMsg(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)

	cmd := a.toast(BadgeOk, "hello")
	if cmd == nil {
		t.Fatal("toast should return a tick Cmd that schedules the clear")
	}
	if !a.toastState.active || a.toastState.text != "hello" {
		t.Fatalf("toast should arm the slot, got active=%v text=%q", a.toastState.active, a.toastState.text)
	}
	if got := a.renderToast(); got == "" {
		t.Fatal("an active toast should render a non-empty line")
	}

	// The tick's payload carries the live seq; delivering it clears the toast.
	seq := a.toastState.seq
	a.Update(clearToastMsg{seq: seq})
	if a.toastState.active || a.toastState.text != "" {
		t.Fatalf("the matching clearToastMsg should clear the toast, got active=%v text=%q", a.toastState.active, a.toastState.text)
	}
	if got := a.renderToast(); got != "" {
		t.Fatalf("a cleared toast should render nothing, got %q", got)
	}
}

// TestToastStaleTickDoesNotClearNewer: a second toast supersedes the first and
// bumps seq; the FIRST toast's pending tick (older seq) must not wipe the newer
// toast when it finally lands.
func TestToastStaleTickDoesNotClearNewer(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)

	_ = a.toast(BadgeOk, "first")
	staleSeq := a.toastState.seq
	_ = a.toast(BadgeWarn, "second")

	// The first toast's tick fires late, carrying the stale seq.
	a.Update(clearToastMsg{seq: staleSeq})
	if !a.toastState.active || a.toastState.text != "second" {
		t.Fatalf("a stale tick must not clear the newer toast, got active=%v text=%q", a.toastState.active, a.toastState.text)
	}

	// The newer toast's own tick (current seq) still clears it.
	a.Update(clearToastMsg{seq: a.toastState.seq})
	if a.toastState.active {
		t.Fatal("the current-seq tick should clear the newer toast")
	}
}

// TestCtrlCArmsThenQuitsWhenIdle: with no run active, ^C opens the
// quit-confirm overlay instead of arming a double-tap.
func TestCtrlCArmsThenQuitsWhenIdle(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)

	cmd, intercepted := a.handleKey(ctrl('c'))
	if !intercepted {
		t.Fatal("ctrl+c must be intercepted")
	}
	if isQuitCmd(cmd) {
		t.Fatal("idle ^C must NOT quit directly — it opens the confirm overlay")
	}
	if a.overlay.Mode() != modeQuitConfirm {
		t.Fatal("idle ^C should open the quit-confirm overlay")
	}
}

// TestCtrlCDisarmedByOtherKey: with the quit-confirm overlay open, any key
// other than y/n/esc is handled by the overlay. Pressing 'n' cancels.
func TestCtrlCDisarmedByOtherKey(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)

	if _, intercepted := a.handleKey(ctrl('c')); !intercepted {
		t.Fatal("first idle ^C must be intercepted")
	}
	if a.overlay.Mode() != modeQuitConfirm {
		t.Fatal("idle ^C should open the quit-confirm overlay")
	}

	// Pressing 'n' cancels the quit-confirm overlay.
	_, _ = a.handleKey(press('n'))
	if a.overlay.IsOpen() {
		t.Error("'n' should close the quit-confirm overlay")
	}
}

// TestCtrlCDisarmsViaTick: since ctrl+c now opens the quit-confirm overlay
// instead of arming, the disarm tick is a no-op (quitArmed stays false).
func TestCtrlCDisarmsViaTick(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)

	_, _ = a.handleKey(ctrl('c'))
	if a.overlay.Mode() != modeQuitConfirm {
		t.Fatal("first ^C should open quit-confirm overlay")
	}
	// Close the overlay so the disarm tick can be tested in idle state.
	a.overlay.Close()

	seq := a.quitSeq
	a.Update(disarmQuitMsg{seq: seq})
	// quitArmed should remain false since we never armed.
	if a.quitArmed {
		t.Fatal("quitArmed should remain false (no arming in new flow)")
	}
}

// TestCtrlCStaleDisarmIgnored: since ctrl+c now opens the quit-confirm overlay
// instead of arming, stale disarm ticks are harmless no-ops.
func TestCtrlCStaleDisarmIgnored(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)

	_, _ = a.handleKey(ctrl('c')) // opens overlay
	staleSeq := a.quitSeq
	a.overlay.Close()

	// A stale disarm tick should be harmless.
	a.Update(disarmQuitMsg{seq: staleSeq})
	if a.quitArmed {
		t.Fatal("quitArmed should remain false (no arming in new flow)")
	}
}

// TestCtrlCCancelsNotQuitsWhileRunning: with a run active, ^C cancels the run
// (current behavior) and must NOT quit and must NOT arm — the double-tap guard
// only governs the idle quit.
func TestCtrlCCancelsNotQuitsWhileRunning(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)

	// Simulate an in-flight run: a live cancel func + the running flag, exactly
	// as runAgent sets them.
	ctx, cancel := context.WithCancel(context.Background())
	cancelled := false
	a.runMu.Lock()
	a.runCtx = ctx
	a.runCancel = func() { cancelled = true; cancel() }
	a.running = true
	a.runMu.Unlock()

	cmd, intercepted := a.handleKey(ctrl('c'))
	if !intercepted {
		t.Fatal("ctrl+c must be intercepted while running")
	}
	if isQuitCmd(cmd) {
		t.Fatal("ctrl+c while a run is active must cancel, not quit")
	}
	if a.quitArmed {
		t.Fatal("ctrl+c while running must NOT arm the idle quit guard")
	}
	if !cancelled {
		t.Fatal("ctrl+c while running should invoke the run's cancel func")
	}
}
