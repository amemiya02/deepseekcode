// placeholder.go drives the dynamic textarea placeholder. The hint reflects
// state: a "Working…" caption
// while a run is active, otherwise one of a small rotation of friendly "Ready"
// hints. The rotation is deterministic — indexed by a turn counter, never a
// clock or random source — so golden/snapshot tests never flap.
package tui

// readyHints rotate while idle so the empty prompt feels alive without ever
// depending on wall-clock time. Each carries the core keybinding reminders the
// old static placeholder showed, so no affordance is lost in the rotation.
var readyHints = []string{
	"Ask anything…  (⏎ send · ⇧⏎ newline · ^D quit · /help)",
	"Type / for commands · @ to mention a file · ↑ recalls history",
	"Ready when you are…  (⏎ send · ⇧⏎ newline · ^P palette)",
	"What should we build next?  (/help for commands · ? for keys)",
}

// workingPlaceholder is shown whenever a run is active. It is intentionally
// static (no rotation) so it reads as a single steady "busy" state rather than
// competing with the chrome spinner for the eye.
const workingPlaceholder = "Working…  (queue a follow-up with ⏎ · ^C to cancel)"

// placeholderFor returns the placeholder string for the given state. While a
// run is active it is the steady working caption; otherwise it is the idle hint
// selected from readyHints by turn (mod len), so consecutive turns rotate
// through the set deterministically. turn is clamped to non-negative.
func placeholderFor(running bool, turn int) string {
	if running {
		return workingPlaceholder
	}
	if turn < 0 {
		turn = 0
	}
	return readyHints[turn%len(readyHints)]
}

// refreshPlaceholder recomputes the textarea placeholder from the live run
// state and the idle-hint rotation counter. It is called whenever the running
// flag flips (a run starts or finishes) so the caption tracks state. The idle
// rotation advances once per completed turn (incremented at the Done path), so
// each fresh idle prompt shows the next hint in the rotation.
func (a *App) refreshPlaceholder() {
	a.input.Placeholder = placeholderFor(a.uiRunning, a.placeholderTurn)
}
