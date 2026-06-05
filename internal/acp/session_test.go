package acp_test

import (
	"context"
	"errors"
	"testing"
	"time"

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

func (s *stubAgent) Steer(_ string) {}

func TestSessionManagerCreateAndLookup(t *testing.T) {
	sm := acp.NewSessionManager(stubAgentFactory)
	id, err := sm.NewSession(context.Background(), "/tmp")
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("expected non-empty session id")
	}
	sess, err := sm.Lookup(id)
	if err != nil {
		t.Fatalf("expected session to exist after creation, got error: %v", err)
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
	_, err := sm.Lookup(id)
	if !errors.Is(err, acp.ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound after cancel, got %v", err)
	}
}

// TestSessionCtxOutlivesCreatingContext verifies the lifecycle fix: a session's
// context is parented to the manager's long-lived base context, NOT to the
// per-request context passed to NewSession. Cancelling the creating context must
// NOT cancel a logically-alive session.
func TestSessionCtxOutlivesCreatingContext(t *testing.T) {
	sm := acp.NewSessionManager(stubAgentFactory)
	reqCtx, reqCancel := context.WithCancel(context.Background())
	id, err := sm.NewSession(reqCtx, "/tmp")
	if err != nil {
		t.Fatal(err)
	}
	// Simulate the creating request/connection closing.
	reqCancel()

	sessCtx := sm.SessionCtx(id)
	select {
	case <-sessCtx.Done():
		t.Fatal("session context was cancelled when the creating context closed; " +
			"it must stay alive until Cancel()")
	default:
		// expected: session still alive
	}

	// The only intentional cancellation path is Cancel().
	sm.Cancel(id)
}

// steerStub implements AgentRunner and records calls to Steer.
type steerStub struct {
	steer func(string)
}

func (s *steerStub) Run(_ context.Context, _ string, _ func(acp.AgentEvent)) error {
	return nil
}

func (s *steerStub) Steer(text string) {
	if s.steer != nil {
		s.steer(text)
	}
}

func TestSessionManagerSteer(t *testing.T) {
	var got []string
	factory := func(string) (acp.AgentRunner, error) {
		return &steerStub{steer: func(s string) { got = append(got, s) }}, nil
	}
	sm := acp.NewSessionManager(factory)
	id, err := sm.NewSession(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	sm.Steer(id, "focus here")
	sm.Steer("sess-unknown", "ignored") // no-op, must not panic
	if len(got) != 1 || got[0] != "focus here" {
		t.Fatalf("expected [focus here], got %#v", got)
	}
}

// TestSessionManagerSteerCancelRace is the race-detector counterpart to
// TestSessionManagerSteer. It exercises concurrent Steer + Cancel on the same
// session from two separate goroutines, verifying that the SessionManager's
// mutex protects the sessions map under the -race detector. Neither call must
// panic or produce a data race even when they interleave at the map level.
func TestSessionManagerSteerCancelRace(t *testing.T) {
	// blockingFactory returns a runner whose Run blocks until ctx is cancelled,
	// giving the test goroutines a live session to race against.
	blockingFactory := func(string) (acp.AgentRunner, error) {
		return &blockingSteerCancel{}, nil
	}
	sm := acp.NewSessionManager(blockingFactory)
	id, err := sm.NewSession(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}

	// Start the run in a goroutine so the session is live.
	runDone := make(chan struct{})
	go func() {
		defer close(runDone)
		// Prompt drives the runner; it will return once Cancel cancels the context.
		_ = sm.Prompt(sm.SessionCtx(id), id, "start", func(acp.AgentEvent) {})
	}()

	// Give Run a moment to enter the runner.
	time.Sleep(5 * time.Millisecond)

	// Fire Steer and Cancel concurrently from two goroutines — the race detector
	// flags any unsynchronised access to the sessions map.
	done := make(chan struct{}, 2)
	go func() {
		sm.Steer(id, "concurrent steer")
		done <- struct{}{}
	}()
	go func() {
		sm.Cancel(id)
		done <- struct{}{}
	}()
	<-done
	<-done

	// Wait for Run to finish (Cancel cancelled the context).
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not finish after Cancel")
	}
}

// blockingSteerCancel is an AgentRunner whose Run blocks until ctx is done.
type blockingSteerCancel struct{}

func (b *blockingSteerCancel) Run(ctx context.Context, _ string, _ func(acp.AgentEvent)) error {
	<-ctx.Done()
	return ctx.Err()
}

func (b *blockingSteerCancel) Steer(_ string) {}

// TestSessionCtxMissingIsCancelled verifies the footgun fix: SessionCtx for an
// unknown/removed session returns an already-cancelled context, not a live
// context.Background(), so a racing caller never launches an uncancellable
// agent goroutine.
func TestSessionCtxMissingIsCancelled(t *testing.T) {
	sm := acp.NewSessionManager(stubAgentFactory)
	ctx := sm.SessionCtx("does-not-exist")
	select {
	case <-ctx.Done():
		// expected: already cancelled
	default:
		t.Fatal("SessionCtx for a missing session must return an already-cancelled context")
	}
}
