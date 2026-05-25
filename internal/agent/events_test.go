package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// TestEventsChannelInitialized confirms New() wires up a non-nil
// buffered event channel. Buffered so streaming token bursts don't
// block the agent goroutine on an idle consumer.
func TestEventsChannelInitialized(t *testing.T) {
	a := New(nil, tools.New(), permissions.New(permissions.ModeReadOnly, "", nil, nil, nil), "x")
	ch := a.Events()
	if ch == nil {
		t.Fatal("Events() must return a non-nil channel")
	}
	if cap(a.eventsCompat) < 1 {
		t.Errorf("events channel should be buffered (cap > 0); got %d", cap(a.eventsCompat))
	}
}

// TestEmitInfoPushesEvent verifies the EmitInfo helper pushes an
// EventInfo onto the stream. This is the path llm.Client.OnRetry
// uses to surface retry notices through the same UI channel as
// agent-emitted events.
func TestEmitInfoPushesEvent(t *testing.T) {
	a := New(nil, tools.New(), permissions.New(permissions.ModeReadOnly, "", nil, nil, nil), "x")
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
	case <-time.After(time.Second):
		t.Fatal("EmitInfo did not push an event")
	}
}

// TestRunEmitsEventDoneOnExit pins the load-bearing contract: every
// Run terminates with an EventDone on the event channel, even on
// early-exit paths (ctx cancel, stream error, etc.). The TUI relies
// on EventDone arriving AFTER every other event to reset the chrome
// caption — bypassing the channel for completion used to race past
// trailing tokens and leave chrome stuck on "writing…".
func TestRunEmitsEventDoneOnExit(t *testing.T) {
	a := New(nil, tools.New(), permissions.New(permissions.ModeReadOnly, "", nil, nil, nil), "x")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel: Run's first select fires immediately
	go func() { _, _ = a.Run(ctx, "") }()

	select {
	case ev := <-a.Events():
		done, ok := ev.(EventDone)
		if !ok {
			t.Fatalf("expected EventDone, got %T", ev)
		}
		if done.Reason != StopContextCancel {
			t.Errorf("Reason mismatch: got %v, want %v", done.Reason, StopContextCancel)
		}
		if !errors.Is(done.Err, context.Canceled) {
			t.Errorf("Err should wrap context.Canceled; got %v", done.Err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("EventDone never arrived on the channel")
	}
}
