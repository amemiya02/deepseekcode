package agent

import (
	"testing"

	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// TestEventsChannelInitialized confirms New() wires up a non-nil
// buffered event channel. Buffered so streaming token bursts don't
// block the agent goroutine on an idle consumer.
func TestEventsChannelInitialized(t *testing.T) {
	a := New(nil, tools.New(), permissions.New(permissions.ModeReadOnly, "", nil, nil), "x")
	ch := a.Events()
	if ch == nil {
		t.Fatal("Events() must return a non-nil channel")
	}
	if cap(a.events) < 1 {
		t.Errorf("events channel should be buffered (cap > 0); got %d", cap(a.events))
	}
}

// TestEmitInfoPushesEvent verifies the EmitInfo helper pushes an
// EventInfo onto the stream. This is the path llm.Client.OnRetry
// uses to surface retry notices through the same UI channel as
// agent-emitted events.
func TestEmitInfoPushesEvent(t *testing.T) {
	a := New(nil, tools.New(), permissions.New(permissions.ModeReadOnly, "", nil, nil), "x")
	a.EmitInfo("hello")
	select {
	case ev := <-a.Events():
		info, ok := ev.(EventInfo)
		if !ok {
			t.Fatalf("expected EventInfo, got %T", ev)
		}
		if info.Text != "hello" {
			t.Errorf("EventInfo.Text mismatch: got %q", info.Text)
		}
	default:
		t.Fatal("EmitInfo should push an event synchronously")
	}
}
