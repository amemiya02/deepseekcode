package agent

import (
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/tools"
)

// Regression: the GUI gateway consumes the Bus directly (acp adapter) and never
// calls Events(). The compat bridge used to forward into the buffered
// eventsCompat channel with a BLOCKING send; once ~512 events accumulated with
// no Events() consumer, the bridge goroutine parked forever, its subscription
// buffer filled, and the next reply-carrying publish (EventPermissionAsk —
// delivered blocking to EVERY subscriber) wedged the agent goroutine. Symptom:
// ask-mode tool approval spins forever in the GUI with no approval UI anywhere.
// The bridge now forwards with ring semantics (drop-oldest when full) so it can
// never park.
func TestPermissionPublishWithoutEventsConsumer(t *testing.T) {
	a := New(nil, tools.New(), nil, "")

	// A gateway-adapter-style consumer: subscribes to the Bus and drains.
	sub := a.Bus().Subscribe(4096)
	go func() {
		for range sub.C {
		}
	}()

	done := make(chan struct{})
	go func() {
		// Overflow the legacy compat path: eventsCompat (256) + bridge sub (256).
		for i := 0; i < 600; i++ {
			a.Bus().Publish(EventTextDelta{Text: "x"})
		}
		// The reply-carrying publish must not block on the unconsumed bridge.
		reply := make(chan PermissionResponse, 1)
		a.Bus().Publish(EventPermissionAsk{Reply: reply})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Bus.Publish(EventPermissionAsk) wedged on the unconsumed Events() compat bridge")
	}
}

// The ring bridge must preserve the legacy contract: a consumer that calls
// Events() (TUI / CLI / spawn drainer) still receives events published
// after the call.
func TestEventsCompatStillDelivers(t *testing.T) {
	a := New(nil, tools.New(), nil, "")
	ch := a.Events()
	a.Bus().Publish(EventInfo{Text: "hello"})
	select {
	case ev := <-ch:
		inf, ok := ev.(EventInfo)
		if !ok || inf.Text != "hello" {
			t.Fatalf("Events() delivered %#v, want EventInfo{hello}", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Events() compat bridge delivered nothing")
	}
}

// Construction-time capture: events published BEFORE the first Events() call
// are buffered and delivered (several agent tests and consumers read Events()
// only after acting). This is the contract a lazily-installed bridge broke.
func TestEventsCompatCapturesPrePublishedEvents(t *testing.T) {
	a := New(nil, tools.New(), nil, "")
	a.Bus().Publish(EventInfo{Text: "early"})
	select {
	case ev := <-a.Events():
		inf, ok := ev.(EventInfo)
		if !ok || inf.Text != "early" {
			t.Fatalf("Events() delivered %#v, want EventInfo{early}", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("event published before the first Events() call was lost")
	}
}
