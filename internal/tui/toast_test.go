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

// TestCtrlCArmsThenQuitsWhenIdle: with no run active, the first ^C arms quit
// (no quit yet, a warning toast) and the second ^C within the window quits.
func TestCtrlCArmsThenQuitsWhenIdle(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)

	cmd, intercepted := a.handleKey(ctrl('c'))
	if !intercepted {
		t.Fatal("ctrl+c must be intercepted")
	}
	if isQuitCmd(cmd) {
		t.Fatal("the first idle ^C must NOT quit — it arms")
	}
	if !a.quitArmed {
		t.Fatal("the first idle ^C should arm quit")
	}
	if !a.toastState.active || a.toastState.text != "press ^C again to quit" {
		t.Fatalf("the first idle ^C should show the arm toast, got active=%v text=%q", a.toastState.active, a.toastState.text)
	}

	cmd2, intercepted2 := a.handleKey(ctrl('c'))
	if !intercepted2 {
		t.Fatal("the second ^C must be intercepted")
	}
	if !isQuitCmd(cmd2) {
		t.Fatal("the second armed ^C should quit")
	}
}

// TestCtrlCDisarmedByOtherKey: an armed idle-quit is cancelled by ANY non-^C
// key — the prompt is dismissed, the arm dropped, and a later lone ^C re-arms
// (asks again) rather than quitting on a stale arm. This is the reported fix:
// pressing anything other than ^C must clear the "press ^C again" prompt.
func TestCtrlCDisarmedByOtherKey(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)

	if _, intercepted := a.handleKey(ctrl('c')); !intercepted {
		t.Fatal("first idle ^C must be intercepted")
	}
	if !a.quitArmed || !a.toastState.active {
		t.Fatal("first idle ^C should arm quit and show the prompt")
	}

	// Any other key disarms and dismisses the toast.
	_, _ = a.handleKey(press('j'))
	if a.quitArmed {
		t.Error("a non-^C key must disarm the idle-quit")
	}
	if a.toastState.active {
		t.Error("a non-^C key must dismiss the 'press ^C again to quit' toast")
	}

	// A ^C after the disarm must re-arm (ask again), NOT quit on a stale flag.
	cmd, _ := a.handleKey(ctrl('c'))
	if isQuitCmd(cmd) {
		t.Error("^C after disarming must re-arm, not quit")
	}
	if !a.quitArmed {
		t.Error("^C after disarming should re-arm the idle-quit")
	}
}

// TestCtrlCDisarmsViaTick: the arm clears when the disarm tick lands, so a
// later lone ^C re-arms (does not quit) rather than quitting on a stale flag.
func TestCtrlCDisarmsViaTick(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)

	_, _ = a.handleKey(ctrl('c'))
	if !a.quitArmed {
		t.Fatal("first ^C should arm")
	}
	seq := a.quitSeq

	a.Update(disarmQuitMsg{seq: seq})
	if a.quitArmed {
		t.Fatal("the matching disarm tick should drop the arm")
	}

	// A fresh ^C re-arms (does not quit) because the flag was disarmed.
	cmd, _ := a.handleKey(ctrl('c'))
	if isQuitCmd(cmd) {
		t.Fatal("a ^C after disarm must re-arm, not quit")
	}
	if !a.quitArmed {
		t.Fatal("a ^C after disarm should re-arm")
	}
}

// TestCtrlCStaleDisarmIgnored: a re-arm bumps quitSeq, so the previous arm's
// disarm tick (stale seq) must not cancel the fresh arm.
func TestCtrlCStaleDisarmIgnored(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)

	_, _ = a.handleKey(ctrl('c')) // arm #1
	staleSeq := a.quitSeq
	// Disarm #1, then re-arm: the new arm carries a fresh seq.
	a.Update(disarmQuitMsg{seq: staleSeq})
	_, _ = a.handleKey(ctrl('c')) // arm #2 (re-arm)

	// Arm #1's tick fires late with the stale seq — must not disarm arm #2.
	a.Update(disarmQuitMsg{seq: staleSeq})
	if !a.quitArmed {
		t.Fatal("a stale disarm tick must not cancel the current arm")
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
