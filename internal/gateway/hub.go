package gateway

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/amemiya02/deepseekcode/internal/acp"
)

// sseEvent is one named SSE frame queued for a subscriber.
// name is the SSE event name ("delta", "tool", "step", "done"); data is the
// raw payload written after "data: ".
type sseEvent struct {
	name string
	data string
}

// subscriber is one connected /v1/events client for a session.
type subscriber struct {
	ch  chan sseEvent
	ctx context.Context
}

// hub is a minimal per-session SSE fan-out. It mirrors the broadcast
// discipline of acp.HTTPGateway (buffered channels, non-blocking sends that
// drop on a stalled client) but speaks the SPA's named-event vocabulary rather
// than the ACP JSON-RPC notification shape.
type hub struct {
	mu      sync.Mutex
	clients map[string][]*subscriber // sessionID -> subscribers
}

func newHub() *hub {
	return &hub{clients: make(map[string][]*subscriber)}
}

// subscribe registers a subscriber for sessionID. The returned unsubscribe
// function removes it. ctx is the request context; broadcast skips a client
// whose ctx is done.
func (h *hub) subscribe(sessionID string, ctx context.Context) (*subscriber, func()) {
	sub := &subscriber{ch: make(chan sseEvent, 64), ctx: ctx}
	h.mu.Lock()
	h.clients[sessionID] = append(h.clients[sessionID], sub)
	h.mu.Unlock()

	unsub := func() {
		h.mu.Lock()
		list := h.clients[sessionID]
		for i, c := range list {
			if c == sub {
				h.clients[sessionID] = append(list[:i], list[i+1:]...)
				break
			}
		}
		if len(h.clients[sessionID]) == 0 {
			delete(h.clients, sessionID)
		}
		h.mu.Unlock()
	}
	return sub, unsub
}

// broadcast delivers ev to every current subscriber of sessionID. Sends are
// non-blocking: a subscriber whose 64-slot buffer is full has the frame dropped
// rather than stalling the agent goroutine for every other client.
func (h *hub) broadcast(sessionID string, ev sseEvent) {
	h.mu.Lock()
	clients := make([]*subscriber, len(h.clients[sessionID]))
	copy(clients, h.clients[sessionID])
	h.mu.Unlock()
	for _, c := range clients {
		select {
		case <-c.ctx.Done():
		case c.ch <- ev:
		default:
		}
	}
}

// mapAgentEvent translates an acp.AgentEvent into a named SSE frame using the
// spec §8.1 event vocabulary. Structured kinds carry a JSON object as data so
// the SPA can parse fields; the legacy text kinds carry the raw text. The
// permission_request / ask_request frames are NOT produced here — the gateway
// emits those itself after registering the interaction in its pending map (it
// needs to inject the assigned id), so this function only covers the
// fire-and-forget kinds.
func mapAgentEvent(ev acp.AgentEvent) sseEvent {
	switch ev.Kind {
	case acp.EventKindTextDelta:
		return sseEvent{name: "message_delta", data: mustJSON(map[string]any{
			"text": ev.Text,
		})}
	case acp.EventKindToolStart:
		// args must be an embedded JSON object, not a string. Unmarshal the raw
		// JSON string from ToolArgs so it is embedded as a nested object.
		var argsObj any
		if err := json.Unmarshal([]byte(ev.ToolArgs), &argsObj); err != nil || argsObj == nil {
			argsObj = map[string]any{}
		}
		return sseEvent{name: "tool_start", data: mustJSON(map[string]any{
			"id": ev.ToolCallID, "name": ev.ToolName, "args": argsObj, "read_only": ev.ToolReadOnly,
		})}
	case acp.EventKindToolEnd:
		return sseEvent{name: "tool_end", data: mustJSON(map[string]any{
			"id": ev.ToolCallID, "result": ev.ToolResult, "is_error": ev.ToolIsErr,
		})}
	case acp.EventKindDone:
		stopReason := ev.StopReason
		if ev.Err != nil {
			stopReason = "error: " + ev.Err.Error()
		}
		return sseEvent{name: "turn_done", data: mustJSON(map[string]any{
			"stop_reason": stopReason,
		})}
	case acp.EventKindCache:
		return sseEvent{name: "cache_update", data: mustJSON(map[string]any{
			"turn_pct": ev.TurnPct, "avg_pct": ev.AvgPct,
			"prefixes": ev.Prefixes, "eviction": ev.Eviction,
		})}
	case acp.EventKindCost:
		return sseEvent{name: "cost_update", data: mustJSON(map[string]any{
			"turn_cny": ev.TurnCNY, "session_cny": ev.SessionCNY, "output_tokens": ev.OutputTokens,
		})}
	case acp.EventKindRouting:
		return sseEvent{name: "routing", data: mustJSON(map[string]any{
			"from": ev.From, "to": ev.To, "reason": ev.Reason,
		})}
	case acp.EventKindJob:
		return sseEvent{name: "job_update", data: mustJSON(map[string]any{"running": ev.Running})}
	case acp.EventKindRetry:
		return sseEvent{name: "retry", data: mustJSON(map[string]any{"attempt": ev.Attempt, "max": ev.Max})}
	case acp.EventKindThinking:
		return sseEvent{name: "thinking_delta", data: mustJSON(map[string]any{"text": ev.Text})}
	case acp.EventKindToolDelta:
		return sseEvent{name: "tool_delta", data: mustJSON(map[string]any{"id": ev.ToolCallID, "delta": ev.ToolDelta})}
	case acp.EventKindPlan:
		return sseEvent{name: "plan_update", data: mustJSON(map[string]any{"items": ev.Plan})}
	default:
		// EventKindInfo and any unknown kinds: emit as message_delta so the SPA
		// can display them as prose. The legacy "step" event name is not part of
		// Contract 2 and must not be emitted.
		return sseEvent{name: "message_delta", data: mustJSON(map[string]any{
			"text": ev.Text,
		})}
	}
}

// mustJSON marshals v to a compact JSON string; on the (impossible for these
// map[string]any payloads) marshal error it returns "{}".
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
