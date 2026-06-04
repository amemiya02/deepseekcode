package acp

// White-box tests for AgentAdapter. These live in package acp (not acp_test)
// so they can access the unexported agentIface and inject a stub agent without
// requiring a real *agent.Agent (which needs DeepSeek credentials).

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/agent"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
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

func (s *stubAgent) Bus() *agent.Bus                  { return s.bus }
func (s *stubAgent) ToolIsReadOnly(_ string) bool      { return false }

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

// permAskAgent drives AgentAdapter.Run through the EventPermissionAsk path.
// It publishes a single EventPermissionAsk carrying a buffered Reply channel
// (mirroring the real agent), records the response the adapter sends back, then
// publishes EventDone and returns. This faithfully exercises the adapter's
// reply-on-permission behaviour rather than a plain-channel stand-in.
type permAskAgent struct {
	bus     *agent.Bus
	gotResp chan agent.PermissionResponse
}

func newPermAskAgent() *permAskAgent {
	return &permAskAgent{
		bus:     agent.NewBus(),
		gotResp: make(chan agent.PermissionResponse, 1),
	}
}

func (s *permAskAgent) Bus() *agent.Bus               { return s.bus }
func (s *permAskAgent) ToolIsReadOnly(_ string) bool   { return false }

func (s *permAskAgent) Run(ctx context.Context, _ string) (agent.StopReason, error) {
	reply := make(chan agent.PermissionResponse, 1)
	s.bus.Publish(agent.EventPermissionAsk{Reply: reply})
	// The agent goroutine parks on the reply, exactly as the real agent does.
	select {
	case resp := <-reply:
		s.gotResp <- resp
	case <-ctx.Done():
		return agent.StopUnknown, ctx.Err()
	}
	s.bus.Publish(agent.EventDone{Reason: agent.StopModelDone})
	return agent.StopModelDone, nil
}

// TestBusPermissionAskNoDeadlock drives AgentAdapter.Run with an agent that
// asks for permission. The onEvent handler must call Respond to unblock the
// agent; the adapter forwards the event rather than hard-denying it.
func TestBusPermissionAskNoDeadlock(t *testing.T) {
	stub := newPermAskAgent()
	ad := &AgentAdapter{a: stub}

	done := make(chan error, 1)
	go func() {
		done <- ad.Run(context.Background(), "prompt", func(ev AgentEvent) {
			if ev.Kind == EventKindPermission && ev.Respond != nil {
				// Deny: the consumer is responsible for answering.
				ev.Respond(PermissionDeny)
			}
		})
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("AgentAdapter.Run deadlocked on EventPermissionAsk")
	}

	select {
	case resp := <-stub.gotResp:
		if resp.Allow {
			t.Fatalf("adapter replied Allow=true, want Allow=false denial")
		}
	default:
		t.Fatal("adapter never replied to EventPermissionAsk")
	}
}

// closeBeforeDoneAgent drives AgentAdapter.Run through the "bus closed before
// EventDone" path. Its Run closes the bus and returns an error WITHOUT
// publishing EventDone, simulating an agent panic. The adapter must observe the
// closed subscription channel, drain the done channel, and return that error.
type closeBeforeDoneAgent struct {
	bus *agent.Bus
	err error
}

func (s *closeBeforeDoneAgent) Bus() *agent.Bus               { return s.bus }
func (s *closeBeforeDoneAgent) ToolIsReadOnly(_ string) bool   { return false }

func (s *closeBeforeDoneAgent) Run(_ context.Context, _ string) (agent.StopReason, error) {
	// Close the bus mid-run without ever publishing EventDone.
	s.bus.Close()
	return agent.StopUnknown, s.err
}

// TestBusCloseBeforeDone drives AgentAdapter.Run with an agent whose bus closes
// before EventDone is published. Run must return (not hang) and surface the
// error the agent produced.
func TestBusCloseBeforeDone(t *testing.T) {
	wantErr := errors.New("agent panic error")
	stub := &closeBeforeDoneAgent{bus: agent.NewBus(), err: wantErr}
	ad := &AgentAdapter{a: stub}

	done := make(chan error, 1)
	go func() {
		done <- ad.Run(context.Background(), "prompt", func(AgentEvent) {})
	}()

	select {
	case err := <-done:
		if err != wantErr {
			t.Fatalf("Run returned err=%v, want %v", err, wantErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("AgentAdapter.Run hung after bus closed before EventDone")
	}
}

// busAgent is a stub agentIface whose Run publishes a scripted sequence of
// bus events, then EventDone. It lets us drive AgentAdapter without a real LLM.
type busAgent struct {
	bus    *agent.Bus
	script func(b *agent.Bus)
}

func (a *busAgent) Bus() *agent.Bus               { return a.bus }
func (a *busAgent) ToolIsReadOnly(_ string) bool   { return false }
func (a *busAgent) Run(ctx context.Context, userPrompt string) (agent.StopReason, error) {
	a.script(a.bus)
	a.bus.Publish(agent.EventDone{Reason: agent.StopModelDone})
	return agent.StopModelDone, nil
}

func TestAdapterForwardsPermission(t *testing.T) {
	bus := agent.NewBus()
	stub := &busAgent{bus: bus, script: func(b *agent.Bus) {
		reply := make(chan agent.PermissionResponse, 1)
		b.Publish(agent.EventPermissionAsk{
			Check: permissions.Check{Tool: fakeTool{name: "write_file"}, Args: json.RawMessage(`{"path":"a.go"}`)},
			Reply: reply,
		})
		// Block the scripted run until the consumer answers, mirroring the agent.
		resp := <-reply
		if !resp.Allow {
			t.Errorf("expected Allow=true from Respond, got false")
		}
	}}
	ad := &AgentAdapter{a: stub}

	var sawPerm bool
	err := ad.Run(context.Background(), "go", func(ev AgentEvent) {
		if ev.Kind == EventKindPermission {
			sawPerm = true
			if ev.ToolName != "write_file" {
				t.Errorf("ToolName = %q, want write_file", ev.ToolName)
			}
			ev.Respond(PermissionAllowOnce)
		}
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !sawPerm {
		t.Fatal("adapter did not forward EventKindPermission")
	}
}

// fakeTool implements the tools.Tool surface permissions.Check needs (Name()).
type fakeTool struct{ name string }

func (f fakeTool) Name() string                                                    { return f.name }
func (f fakeTool) Description() string                                             { return "" }
func (f fakeTool) Parameters() json.RawMessage                                     { return nil }
func (f fakeTool) Execute(context.Context, json.RawMessage) (tools.Result, error)  { return tools.Result{}, nil }
func (f fakeTool) IsReadOnly() bool                                                { return false }

var _ = time.Second
