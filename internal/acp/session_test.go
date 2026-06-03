package acp_test

import (
	"context"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/acp"
)

// stubAgentFactory returns an AgentRunner that immediately sends one TextDelta then Done.
func stubAgentFactory(workingDir string) (acp.AgentRunner, error) {
	return &stubAgent{}, nil
}

type stubAgent struct{}

func (s *stubAgent) Run(ctx context.Context, userPrompt string, onEvent func(acp.AgentEvent)) error {
	onEvent(acp.AgentEvent{Kind: acp.EventKindTextDelta, Text: "hello " + userPrompt})
	onEvent(acp.AgentEvent{Kind: acp.EventKindDone, StopReason: "end_turn"})
	return nil
}

func TestSessionManagerCreateAndLookup(t *testing.T) {
	sm := acp.NewSessionManager(stubAgentFactory)
	id, err := sm.NewSession(context.Background(), "/tmp")
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("expected non-empty session id")
	}
	sess, ok := sm.Lookup(id)
	if !ok {
		t.Fatal("expected session to exist after creation")
	}
	_ = sess
}

func TestSessionManagerDuplicateNewSession(t *testing.T) {
	sm := acp.NewSessionManager(stubAgentFactory)
	id1, _ := sm.NewSession(context.Background(), "/tmp")
	id2, _ := sm.NewSession(context.Background(), "/tmp")
	if id1 == id2 {
		t.Fatal("expected distinct session ids")
	}
}

func TestSessionManagerCancel(t *testing.T) {
	sm := acp.NewSessionManager(stubAgentFactory)
	id, _ := sm.NewSession(context.Background(), "/tmp")
	sm.Cancel(id)
	_, ok := sm.Lookup(id)
	if ok {
		t.Fatal("expected session to be removed after cancel")
	}
}
