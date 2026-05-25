package agent

import (
	"sync"
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// TestBusFanoutOrder verifies all subscribers receive events in the
// same Seq order.
func TestBusFanoutOrder(t *testing.T) {
	b := NewBus()
	s1 := b.Subscribe(256)
	s2 := b.Subscribe(256)

	b.Publish(EventInfo{Text: "a"})
	b.Publish(EventInfo{Text: "b"})
	b.Publish(EventInfo{Text: "c"})

	for _, s := range []*Subscription{s1, s2} {
		for i, want := range []string{"a", "b", "c"} {
			select {
			case env := <-s.C:
				if env.Seq != uint64(i+1) {
					t.Errorf("subscriber: seq %d, want %d", env.Seq, i+1)
				}
				info, ok := env.Event.(EventInfo)
				if !ok {
					t.Fatalf("expected EventInfo, got %T", env.Event)
				}
				if info.Text != want {
					t.Errorf("Text: got %q, want %q", info.Text, want)
				}
			case <-time.After(time.Second):
				t.Fatalf("timed out waiting for event %d (%q)", i, want)
			}
		}
	}
}

// TestBusSlowConsumerDrops verifies that a slow consumer (full buffer)
// drops non-reply events but does not block other subscribers.
func TestBusSlowConsumerDrops(t *testing.T) {
	b := NewBus()
	slow := b.Subscribe(1) // tiny buffer
	b.Subscribe(256)       // fast subscriber just to exercise path

	// Fill slow's buffer with one event, then publish more.
	b.Publish(EventInfo{Text: "first"})

	// Publish several more without draining slow.
	for i := 0; i < 10; i++ {
		b.Publish(EventInfo{Text: "extra"})
	}

	// Slow should have dropped events.
	if slow.Dropped() == 0 {
		t.Error("slow consumer should have dropped events")
	}
}

// TestBusReplyDelivered verifies that reply-carrying events are
// delivered even when the buffer would otherwise be full.
func TestBusReplyDelivered(t *testing.T) {
	b := NewBus()
	s := b.Subscribe(1)

	// Fill the buffer.
	b.Publish(EventInfo{Text: "fill"})

	// Now publish a reply event — should still arrive (blocking send).
	reply := make(chan PermissionResponse, 1)
	done := make(chan struct{})
	go func() {
		b.Publish(EventPermissionAsk{
			Check: permissions.Check{},
			Reply: reply,
		})
		close(done)
	}()

	// Drain the first event, then the reply should arrive.
	select {
	case <-s.C:
		// first event drained
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first event")
	}

	select {
	case env := <-s.C:
		if _, ok := env.Event.(EventPermissionAsk); !ok {
			t.Errorf("expected EventPermissionAsk, got %T", env.Event)
		}
	case <-time.After(time.Second):
		t.Fatal("reply event was not delivered")
	}

	<-done
}

// TestBusSeqMonotonic verifies Seq numbers are strictly increasing.
func TestBusSeqMonotonic(t *testing.T) {
	b := NewBus()
	s := b.Subscribe(256)

	n := 100
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			b.Publish(EventInfo{Text: "x"})
			wg.Done()
		}()
	}
	wg.Wait()

	seen := make(map[uint64]bool)
	for i := 0; i < n; i++ {
		select {
		case env := <-s.C:
			if env.Seq == 0 {
				t.Error("Seq should start at 1")
			}
			if seen[env.Seq] {
				t.Errorf("duplicate Seq %d", env.Seq)
			}
			seen[env.Seq] = true
		case <-time.After(time.Second):
			t.Fatalf("timed out after receiving %d events", i)
		}
	}

	if len(seen) != n {
		t.Errorf("received %d unique seqs, want %d", len(seen), n)
	}
}

// TestBusUnsubscribeRemoves verifies Unsubscribe removes the subscriber
// and subsequent Publish calls do not send to it.
func TestBusUnsubscribeRemoves(t *testing.T) {
	b := NewBus()
	s := b.Subscribe(256)

	b.Publish(EventInfo{Text: "before"})
	<-s.C

	b.Unsubscribe(s)

	// Publish after unsubscribe — should not panic or block.
	b.Publish(EventInfo{Text: "after"})

	// Channel should be closed.
	_, ok := <-s.C
	if ok {
		t.Error("channel should be closed after Unsubscribe")
	}
}

// TestBusCloseShutsDown verifies Close shuts down all subscribers.
func TestBusCloseShutsDown(t *testing.T) {
	b := NewBus()
	s1 := b.Subscribe(256)
	s2 := b.Subscribe(256)

	b.Close()

	// Both channels should be closed.
	_, ok1 := <-s1.C
	_, ok2 := <-s2.C
	if ok1 || ok2 {
		t.Error("channels should be closed after Bus.Close")
	}

	// Publish after Close should be no-op.
	b.Publish(EventInfo{Text: "after close"})
}

// TestBusReplyQuestionDelivered verifies EventQuestionAsk is also
// treated as a reply event (blocking delivery).
func TestBusReplyQuestionDelivered(t *testing.T) {
	b := NewBus()
	s := b.Subscribe(1)

	b.Publish(EventInfo{Text: "fill"})

	reply := make(chan tools.QuestionResponse, 1)
	done := make(chan struct{})
	go func() {
		b.Publish(EventQuestionAsk{
			Questions: []tools.Question{{Question: "q"}},
			Reply:     reply,
		})
		close(done)
	}()

	<-s.C // drain fill event

	select {
	case env := <-s.C:
		if _, ok := env.Event.(EventQuestionAsk); !ok {
			t.Errorf("expected EventQuestionAsk, got %T", env.Event)
		}
	case <-time.After(time.Second):
		t.Fatal("question event was not delivered")
	}

	<-done
}
