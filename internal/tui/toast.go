package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// toastTTL is how long a transient toast stays on screen before its tick
// clears it. Mirrors crush's DefaultStatusTTL (~4s) — long enough to read a
// short notice, short enough that it never lingers into the next interaction.
const toastTTL = 4 * time.Second

// quitArmWindow is how long the first idle ^C arms "press again to quit"
// before the disarm tick fires. Short by design: a stray ^C shouldn't leave
// the session primed to die for very long.
const quitArmWindow = 2 * time.Second

// toastState is the single transient-notice slot (G8, §6.5). It renders as one
// styled line between the status bar and the input box, colored by kind, and is
// cleared by a tea.Tick after toastTTL. seq guards against an older tick wiping
// a newer toast: each toast() call bumps seq and the tick only clears when the
// landing clearToastMsg carries the current seq.
type toastState struct {
	text   string
	kind   BadgeKind
	active bool
	seq    int
}

// clearToastMsg is the deferred clear scheduled by toast(). It carries the seq
// of the toast it was minted for so a stale tick (from a toast that has since
// been replaced) is ignored — only the matching, still-current toast clears.
type clearToastMsg struct{ seq int }

// disarmQuitMsg fires after quitArmWindow to drop the armed-quit flag so a later
// lone ^C cancels/asks again rather than quitting on a long-stale arm. It is a
// no-op when the flag was already cleared (e.g. the user did quit, or a run
// started and re-armed-then-disarmed).
type disarmQuitMsg struct{ seq int }

// toast sets the transient notice and returns the tea.Cmd that schedules its
// expiry. The kind selects the badge color. Each call supersedes any live
// toast and bumps seq so the previous toast's pending tick can no longer clear
// the new one (the guard in Update compares seq).
func (a *App) toast(kind BadgeKind, text string) tea.Cmd {
	a.toastState.seq++
	a.toastState.text = text
	a.toastState.kind = kind
	a.toastState.active = true
	seq := a.toastState.seq
	// Re-layout so the viewport gives up the toast's row; harmless before the
	// first WindowSizeMsg (layout() guards on width==0 downstream of a.width).
	a.layout()
	return tea.Tick(toastTTL, func(time.Time) tea.Msg {
		return clearToastMsg{seq: seq}
	})
}

// clearToastIf clears the toast only when seq matches the live toast's seq, so
// an older tick can never wipe a toast minted after it was scheduled.
func (a *App) clearToastIf(seq int) {
	if a.toastState.seq == seq {
		a.toastState.active = false
		a.toastState.text = ""
		a.layout() // reclaim the toast's row for the viewport
	}
}

// dismissToast clears the live toast immediately rather than waiting for its
// TTL tick, and bumps seq so the pending clear tick becomes a no-op. Used when a
// toast's reason is resolved early — e.g. any non-^C key cancelling the armed
// idle-quit prompt. A no-op when nothing is showing.
func (a *App) dismissToast() {
	if !a.toastState.active {
		return
	}
	a.toastState.seq++
	a.toastState.active = false
	a.toastState.text = ""
	a.layout() // reclaim the toast's row for the viewport
}

// renderToast returns the one-line toast string for View(), or "" when no toast
// is live. The line is a filled badge (so the kind's color reads at a glance)
// preceded by the gutter so it aligns with the rest of the chrome.
func (a *App) renderToast() string {
	if !a.toastState.active || a.toastState.text == "" {
		return ""
	}
	return a.theme.Gutter() + a.theme.Badge(a.toastState.kind).Render(a.toastState.text)
}
