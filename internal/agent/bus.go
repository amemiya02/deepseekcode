package agent

import (
	"sync"
	"sync/atomic"
	"time"
)

// Subscription is one consumer's view of the Bus. C delivers events in
// publish order. Callers that stop reading should Unsubscribe to avoid
// leaking the goroutine that would otherwise block on the full channel.
type Subscription struct {
	C       <-chan EventEnvelope
	c       chan EventEnvelope
	dropped uint64 // atomic
	closed  bool
}

// Dropped returns the number of events dropped for this subscription
// because its buffer was full. Only non-reply events are dropped;
// reply-carrying events block until the consumer reads them.
func (s *Subscription) Dropped() uint64 {
	return atomic.LoadUint64(&s.dropped)
}

// Bus is a multi-consumer fan-out for agent events. Subscribers receive
// every published event as an EventEnvelope in publish order.
//
// Ordinary events are delivered non-blocking: if a subscriber's buffer
// is full the event is dropped and its Dropped counter increments.
// Reply-carrying events (EventPermissionAsk, EventQuestionAsk) are
// delivered blocking — they carry a reply channel the agent goroutine
// parks on, so dropping them would deadlock the agent.
type Bus struct {
	mu     sync.Mutex
	subs   []*Subscription
	seq    uint64
	closed bool
}

// NewBus returns an empty Bus ready for subscribers.
func NewBus() *Bus {
	return &Bus{}
}

// Subscribe adds a consumer and returns its Subscription. buffer is the
// channel capacity; buffer <= 0 defaults to 256 (matching the legacy
// events channel). The caller must drain C or Unsubscribe to avoid
// back-pressure on reply events.
func (b *Bus) Subscribe(buffer int) *Subscription {
	if buffer <= 0 {
		buffer = 256
	}
	s := &Subscription{
		c: make(chan EventEnvelope, buffer),
	}
	s.C = s.c

	b.mu.Lock()
	b.subs = append(b.subs, s)
	b.mu.Unlock()
	return s
}

// Unsubscribe removes the subscription and closes its channel. Safe to
// call multiple times; subsequent calls are no-ops.
func (b *Bus) Unsubscribe(s *Subscription) {
	b.mu.Lock()
	if s.closed {
		b.mu.Unlock()
		return
	}
	s.closed = true
	// Remove from subs list.
	for i, sub := range b.subs {
		if sub == s {
			b.subs = append(b.subs[:i], b.subs[i+1:]...)
			break
		}
	}
	close(s.c)
	b.mu.Unlock()
}

// Publish wraps ev in an EventEnvelope (with a monotonic Seq and
// current time) and fans it out to every subscriber. The caller must
// not hold any locks that a subscriber's reading goroutine might also
// need.
func (b *Bus) Publish(ev Event) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.seq++
	env := EventEnvelope{
		Version: EventProtocolVersion,
		Seq:     b.seq,
		At:      time.Now(),
		Event:   ev,
	}
	// Snapshot subscribers under lock so iteration does not race
	// with Subscribe / Unsubscribe.
	subs := make([]*Subscription, len(b.subs))
	copy(subs, b.subs)
	b.mu.Unlock()

	_, isReply := ev.(EventPermissionAsk)
	if !isReply {
		_, isReply = ev.(EventQuestionAsk)
	}

	for _, s := range subs {
		if isReply {
			// Reply-carrying events must be delivered blocking
			// so the agent goroutine can park on the reply channel.
			// Check closed under lock to avoid sending on a closed
			// channel after Unsubscribe.
			b.mu.Lock()
			closed := s.closed
			b.mu.Unlock()
			if closed {
				continue
			}
			s.c <- env
		} else {
			select {
			case s.c <- env:
			default:
				atomic.AddUint64(&s.dropped, 1)
			}
		}
	}
}

// Close shuts down the Bus and closes every subscriber channel.
// Publish after Close is a no-op.
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for _, s := range b.subs {
		if !s.closed {
			s.closed = true
			close(s.c)
		}
	}
	b.subs = nil
}
