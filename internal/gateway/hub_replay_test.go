package gateway

import (
	"context"
	"testing"
)

// A subscriber that connects AFTER a turn has already emitted frames must still
// receive the whole turn (incl. turn_done) via the replay backlog. This is the
// root-cause guard for the dead-2nd-send bug: without replay, turn_done emitted
// before the SPA's EventSource connects is lost and the composer never unlocks.
func TestHubReplaysBufferedTurnToLateSubscriber(t *testing.T) {
	h := newHub()
	const sid = "s1"
	h.resetTurn(sid)
	h.broadcast(sid, sseEvent{name: "message_delta", data: `{"text":"hi"}`})
	h.broadcast(sid, sseEvent{name: "turn_done", data: `{"stop_reason":"done"}`})

	_, backlog, unsub := h.subscribe(sid, context.Background())
	defer unsub()

	if len(backlog) != 2 {
		t.Fatalf("backlog = %d frames, want 2 (late subscriber must replay the turn)", len(backlog))
	}
	if backlog[0].name != "message_delta" || backlog[1].name != "turn_done" {
		t.Fatalf("backlog order = [%s,%s], want [message_delta,turn_done]", backlog[0].name, backlog[1].name)
	}
}

// resetTurn (called at the start of each prompt) must clear the previous turn's
// frames so a new subscriber does not replay a stale turn_done.
func TestHubResetTurnClearsBuffer(t *testing.T) {
	h := newHub()
	const sid = "s1"
	h.broadcast(sid, sseEvent{name: "message_delta", data: `{}`})
	h.resetTurn(sid)
	_, backlog, unsub := h.subscribe(sid, context.Background())
	defer unsub()
	if len(backlog) != 0 {
		t.Fatalf("backlog = %d, want 0 after resetTurn", len(backlog))
	}
}
