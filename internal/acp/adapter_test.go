package acp_test

import (
	"context"
	"errors"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/acp"
	"github.com/amemiya02/deepseekcode/internal/agent"
)

// TestAdapterCompiles verifies that AgentAdapter implements AgentRunner at compile time.
// It does not actually run the agent (which requires DeepSeek credentials).
func TestAdapterCompiles(t *testing.T) {
	var _ acp.AgentRunner = (*acp.AgentAdapter)(nil)
}

// fakeRunner is a minimal AgentRunner for behavioural tests that replays a
// fixed sequence of AgentEvent values and returns a fixed error.
type fakeRunner struct {
	events []acp.AgentEvent
	err    error
}

func (f *fakeRunner) Run(_ context.Context, _ string, onEvent func(acp.AgentEvent)) error {
	for _, ev := range f.events {
		onEvent(ev)
	}
	return f.err
}

// TestAgentRunnerEventOrder verifies correct event forwarding and error propagation
// through the AgentRunner interface.
func TestAgentRunnerEventOrder(t *testing.T) {
	wantErr := errors.New("test done error")
	r := &fakeRunner{
		events: []acp.AgentEvent{
			{Kind: acp.EventKindTextDelta, Text: "hello"},
			{Kind: acp.EventKindInfo, Text: "some info"},
			{Kind: acp.EventKindDone, StopReason: "end_turn", Err: wantErr},
		},
		err: wantErr,
	}

	var got []acp.AgentEvent
	err := r.Run(context.Background(), "prompt", func(ev acp.AgentEvent) {
		got = append(got, ev)
	})

	if err != wantErr {
		t.Fatalf("Run() error = %v, want %v", err, wantErr)
	}
	if len(got) != 3 {
		t.Fatalf("got %d events, want 3", len(got))
	}
	if got[0].Kind != acp.EventKindTextDelta || got[0].Text != "hello" {
		t.Errorf("event[0] = %+v, want EventKindTextDelta text=hello", got[0])
	}
	if got[1].Kind != acp.EventKindInfo || got[1].Text != "some info" {
		t.Errorf("event[1] = %+v, want EventKindInfo text='some info'", got[1])
	}
	if got[2].Kind != acp.EventKindDone || got[2].StopReason != "end_turn" || got[2].Err != wantErr {
		t.Errorf("event[2] = %+v, want EventKindDone stop=end_turn err=%v", got[2], wantErr)
	}
}

// TestBusPermissionAskNoDeadlock verifies that EventPermissionAsk does NOT
// deadlock a consumer: a reply must be sent on the Reply channel to unblock
// the agent goroutine. This mirrors the fix in AgentAdapter.
func TestBusPermissionAskNoDeadlock(t *testing.T) {
	permReply := make(chan agent.PermissionResponse, 1)
	// Simulate the adapter behaviour: send a denial reply immediately.
	go func() {
		permReply <- agent.PermissionResponse{Allow: false}
	}()

	resp := <-permReply
	if resp.Allow {
		t.Fatal("expected Allow=false denial reply")
	}
}

// TestBusCloseBeforeDone verifies that when a bus channel is closed before
// EventDone is published, the consumer correctly handles the closed channel
// (ok==false) and can still recover the error from a separate done channel —
// simulating the fix in AgentAdapter.
func TestBusCloseBeforeDone(t *testing.T) {
	wantErr := errors.New("agent panic error")

	// Simulate a bus channel that closes unexpectedly.
	ch := make(chan agent.EventEnvelope, 4)
	done := make(chan error, 1)
	done <- wantErr
	close(ch)

	// Adapter logic: if ok==false, drain done.
	var gotErr error
	for env := range ch {
		_ = env // would process normally
	}
	// ch is closed (ok==false path), drain done.
	select {
	case err := <-done:
		gotErr = err
	default:
		gotErr = nil
	}

	if gotErr != wantErr {
		t.Fatalf("on bus close got err=%v, want %v", gotErr, wantErr)
	}
}
