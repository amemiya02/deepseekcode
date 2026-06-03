package acp

// White-box tests for AgentAdapter. These live in package acp (not acp_test)
// so they can access the unexported agentIface and inject a stub agent without
// requiring a real *agent.Agent (which needs DeepSeek credentials).

import (
	"context"
	"errors"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/agent"
)

// stubAgent implements agentIface. It publishes a pre-baked sequence of events
// to a real agent.Bus and then returns a fixed error from Run.
type stubAgent struct {
	bus    *agent.Bus
	events []agent.Event
	err    error
}

func newStubAgent(events []agent.Event, err error) *stubAgent {
	return &stubAgent{
		bus:    agent.NewBus(),
		events: events,
		err:    err,
	}
}

func (s *stubAgent) Bus() *agent.Bus { return s.bus }

// Run publishes all events in order, then emits EventDone (mirroring the real
// Agent.Run defer), then writes to done — matching the production ordering
// guarantee that EventDone arrives before Run's return value.
func (s *stubAgent) Run(_ context.Context, _ string) (agent.StopReason, error) {
	for _, ev := range s.events {
		s.bus.Publish(ev)
	}
	// Publish EventDone last, exactly as the real agent does via defer.
	s.bus.Publish(agent.EventDone{Err: s.err})
	return agent.StopModelDone, s.err
}

// TestAgentAdapterEventMapping drives AgentAdapter.Run with a stubAgent and
// verifies that EventTextDelta, EventInfo, and EventDone are forwarded with
// correct Kind/Text/StopReason/Err values, and that the error from EventDone
// is returned from Run.
func TestAgentAdapterEventMapping(t *testing.T) {
	wantErr := errors.New("done error")

	stub := newStubAgent([]agent.Event{
		agent.EventTextDelta{Text: "hello"},
		agent.EventInfo{Text: "some info"},
	}, wantErr)

	ad := &AgentAdapter{a: stub}

	var got []AgentEvent
	err := ad.Run(context.Background(), "prompt", func(ev AgentEvent) {
		got = append(got, ev)
	})

	if err != wantErr {
		t.Fatalf("Run() error = %v, want %v", err, wantErr)
	}
	if len(got) != 3 {
		t.Fatalf("got %d events, want 3: %+v", len(got), got)
	}
	if got[0].Kind != EventKindTextDelta || got[0].Text != "hello" {
		t.Errorf("event[0] = %+v, want EventKindTextDelta text=hello", got[0])
	}
	if got[1].Kind != EventKindInfo || got[1].Text != "some info" {
		t.Errorf("event[1] = %+v, want EventKindInfo text='some info'", got[1])
	}
	if got[2].Kind != EventKindDone || got[2].Err != wantErr {
		t.Errorf("event[2] = %+v, want EventKindDone err=%v", got[2], wantErr)
	}
}
