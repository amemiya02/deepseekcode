package acp_test

import (
	"context"
	"errors"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/acp"
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

func (f *fakeRunner) Steer(_ string) {}

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
