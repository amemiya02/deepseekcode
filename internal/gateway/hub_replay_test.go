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

// Task-3 diagnostic result (2026-06-05): a live SSE trace of `dsc serve` with
// prompt "你好" confirmed that:
//   - reasoning text arrives as  event: thinking_delta  (EventKindThinking)
//   - answer text arrives as     event: message_delta   (EventKindTextDelta)
//   - turn_done fires last       event: turn_done
//
// The adapter mapping in internal/acp/adapter.go is therefore correct:
//   agent.EventTextDelta      → EventKindTextDelta  (adapter.go:68-69)
//   agent.EventReasoningDelta → EventKindThinking   (adapter.go:130-131)
//
// The "answer renders as Thinking" symptom was the lost-turn_done race: when a
// fast reply finished before the SPA's EventSource connected, turn_done was
// dropped so the UI never promoted the thinking block into a completed answer
// bubble. Tasks 1 (hub replay buffer) and 2 (subscribe-before-dispatch) fix the
// race; this comment records the diagnostic evidence and closes the loop.

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
