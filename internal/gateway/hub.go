package gateway

import (
	"context"
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

// mapAgentEvent translates an acp.AgentEvent into a named SSE frame for the
// SPA. The SPA listens for "delta" (text token), "tool" (tool name), "step"
// (step summary) and "done" (end). The acp layer collapses tool-call and
// step-finish bus events into EventKindInfo before they reach this layer, so
// Info maps to "step"; an explicit tool event would map to "tool" if the acp
// AgentEvent vocabulary grows one.
func mapAgentEvent(ev acp.AgentEvent) sseEvent {
	switch ev.Kind {
	case acp.EventKindTextDelta:
		return sseEvent{name: "delta", data: ev.Text}
	case acp.EventKindInfo:
		return sseEvent{name: "step", data: ev.Text}
	case acp.EventKindDone:
		data := ev.StopReason
		if ev.Err != nil {
			data = "error: " + ev.Err.Error()
		}
		return sseEvent{name: "done", data: data}
	default:
		return sseEvent{name: "step", data: ev.Text}
	}
}
