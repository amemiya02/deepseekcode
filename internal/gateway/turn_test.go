package gateway_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/acp"
	"github.com/amemiya02/deepseekcode/internal/gateway"
)

func TestPermissionRoundTrip(t *testing.T) {
	sm := acp.NewSessionManager(askingAgentFactory)
	h := gateway.NewHandler(sm, "")
	ts := httptest.NewServer(h)
	defer ts.Close()

	resp, _ := http.Post(ts.URL+"/v1/prompt", "application/json", strings.NewReader(`{"prompt":"hi"}`))
	var first struct{ SessionID string `json:"session_id"` }
	json.NewDecoder(resp.Body).Decode(&first)
	resp.Body.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		ts.URL+"/v1/events?session_id="+first.SessionID, nil)
	stream, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open event stream: %v", err)
	}
	defer stream.Body.Close()

	go func() {
		body := `{"prompt":"again","session_id":"` + first.SessionID + `"}`
		r, _ := http.Post(ts.URL+"/v1/prompt", "application/json", strings.NewReader(body))
		if r != nil {
			r.Body.Close()
		}
	}()

	// Read frames: capture permission id, POST a decision, then expect turn_done.
	scanner := bufio.NewScanner(stream.Body)
	var lastEvent, permID string
	var sawDone bool
	for scanner.Scan() {
		line := scanner.Text()
		if name, ok := strings.CutPrefix(line, "event: "); ok {
			lastEvent = name
		}
		if d, ok := strings.CutPrefix(line, "data: "); ok {
			switch lastEvent {
			case "permission_request":
				var p struct{ ID string `json:"id"` }
				json.Unmarshal([]byte(d), &p)
				permID = p.ID
				body := `{"id":"` + permID + `","decision":"once"}`
				pr, _ := http.Post(ts.URL+"/v1/permission", "application/json", strings.NewReader(body))
				if pr != nil {
					if pr.StatusCode != http.StatusOK {
						t.Errorf("POST /v1/permission: got %d", pr.StatusCode)
					}
					pr.Body.Close()
				}
			case "turn_done":
				sawDone = true
			}
		}
		if sawDone {
			break
		}
	}
	if permID == "" {
		t.Fatal("never saw a permission id")
	}
	if !sawDone {
		t.Fatal("turn never completed after resolving permission")
	}
}

func TestAskRoundTrip(t *testing.T) {
	sm := acp.NewSessionManager(questionAgentFactory)
	h := gateway.NewHandler(sm, "")
	ts := httptest.NewServer(h)
	defer ts.Close()

	resp, _ := http.Post(ts.URL+"/v1/prompt", "application/json", strings.NewReader(`{"prompt":"hi"}`))
	var first struct{ SessionID string `json:"session_id"` }
	json.NewDecoder(resp.Body).Decode(&first)
	resp.Body.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		ts.URL+"/v1/events?session_id="+first.SessionID, nil)
	stream, err2 := http.DefaultClient.Do(req)
	if err2 != nil {
		t.Fatalf("open event stream: %v", err2)
	}
	defer stream.Body.Close()

	go func() {
		body := `{"prompt":"again","session_id":"` + first.SessionID + `"}`
		r, _ := http.Post(ts.URL+"/v1/prompt", "application/json", strings.NewReader(body))
		if r != nil {
			r.Body.Close()
		}
	}()

	scanner := bufio.NewScanner(stream.Body)
	var lastEvent, askID string
	var sawDone bool
	for scanner.Scan() {
		line := scanner.Text()
		if name, ok := strings.CutPrefix(line, "event: "); ok {
			lastEvent = name
		}
		if d, ok := strings.CutPrefix(line, "data: "); ok {
			switch lastEvent {
			case "ask_request":
				var p struct{ ID string `json:"id"` }
				json.Unmarshal([]byte(d), &p)
				askID = p.ID
				body := `{"id":"` + askID + `","answers":[["A"]]}`
				ar, _ := http.Post(ts.URL+"/v1/answer", "application/json", strings.NewReader(body))
				if ar != nil {
					if ar.StatusCode != http.StatusOK {
						t.Errorf("POST /v1/answer: got %d", ar.StatusCode)
					}
					ar.Body.Close()
				}
			case "turn_done":
				sawDone = true
			}
		}
		if sawDone {
			break
		}
	}
	if askID == "" {
		t.Fatal("never saw an ask id")
	}
	if !sawDone {
		t.Fatal("turn never completed after answering")
	}
}

func TestCancelUnknownSession(t *testing.T) {
	sm := acp.NewSessionManager(stubAgentFactory)
	h := gateway.NewHandler(sm, "")
	ts := httptest.NewServer(h)
	defer ts.Close()
	// Cancel is idempotent: unknown id is a no-op 200, not an error.
	r, _ := http.Post(ts.URL+"/v1/cancel", "application/json", strings.NewReader(`{"session_id":"nope"}`))
	if r.StatusCode != http.StatusOK {
		t.Fatalf("cancel unknown: got %d, want 200", r.StatusCode)
	}
	r.Body.Close()
}

// TestSteerEndpoint verifies POST /v1/steer is wired and behaves correctly:
//   - known session: 200, steer reaches the runner
//   - unknown session: 200 (idempotent no-op, like cancel)
func TestSteerEndpoint(t *testing.T) {
	agent := &recordingSteerAgent{}
	factory := func(string) (acp.AgentRunner, error) { return agent, nil }
	sm := acp.NewSessionManager(factory)
	h := gateway.NewHandler(sm, "")
	ts := httptest.NewServer(h)
	defer ts.Close()

	// Create a session.
	resp, err := http.Post(ts.URL+"/v1/prompt", "application/json", strings.NewReader(`{"prompt":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	var first struct {
		SessionID string `json:"session_id"`
	}
	json.NewDecoder(resp.Body).Decode(&first)
	resp.Body.Close()

	// Steer the known session.
	body := `{"session_id":"` + first.SessionID + `","prompt":"focus here"}`
	r, err := http.Post(ts.URL+"/v1/steer", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if r.StatusCode != http.StatusOK {
		t.Fatalf("POST /v1/steer known session: got %d, want 200", r.StatusCode)
	}
	r.Body.Close()
	steered := agent.Steered()
	if len(steered) != 1 || steered[0] != "focus here" {
		t.Fatalf("expected steer to reach runner with 'focus here', got %v", steered)
	}

	// Steer an unknown session is a no-op 200, must not panic.
	r2, err := http.Post(ts.URL+"/v1/steer", "application/json",
		strings.NewReader(`{"session_id":"sess-unknown","prompt":"ignored"}`))
	if err != nil {
		t.Fatal(err)
	}
	if r2.StatusCode != http.StatusOK {
		t.Fatalf("POST /v1/steer unknown session: got %d, want 200", r2.StatusCode)
	}
	r2.Body.Close()
}

// recordingSteerAgent is an AgentRunner that records Steer calls and completes
// its Run immediately, used by TestSteerEndpoint.
// A mutex guards the steered slice so writes from the httptest handler goroutine
// and reads from the test goroutine are race-detector clean.
type recordingSteerAgent struct {
	mu      sync.Mutex
	steered []string
}

func (a *recordingSteerAgent) Run(_ context.Context, _ string, onEvent func(acp.AgentEvent)) error {
	onEvent(acp.AgentEvent{Kind: acp.EventKindDone, StopReason: "end_turn"})
	return nil
}

func (a *recordingSteerAgent) Steer(text string) {
	a.mu.Lock()
	a.steered = append(a.steered, text)
	a.mu.Unlock()
}

// Steered returns a snapshot of the recorded steer calls.
func (a *recordingSteerAgent) Steered() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]string, len(a.steered))
	copy(out, a.steered)
	return out
}

// TestPromptWhileActiveSteers verifies the double-POST race guard: when a turn
// is already in flight for a session, a second POST /v1/prompt routes to Steer
// rather than spawning a concurrent run. Run is called exactly once; the second
// prompt is delivered via Steer.
func TestPromptWhileActiveSteers(t *testing.T) {
	// blockingAgent parks until its release channel is closed, giving the test
	// a window to send a second POST /v1/prompt while the turn is in flight.
	release := make(chan struct{})
	agent := newBlockingSteerAgent(release)

	factory := func(string) (acp.AgentRunner, error) { return agent, nil }
	sm := acp.NewSessionManager(factory)
	h := gateway.NewHandler(sm, "")
	ts := httptest.NewServer(h)
	defer ts.Close()

	// First prompt — starts the blocking turn.
	resp, err := http.Post(ts.URL+"/v1/prompt", "application/json", strings.NewReader(`{"prompt":"first"}`))
	if err != nil {
		t.Fatal(err)
	}
	var first struct {
		SessionID string `json:"session_id"`
	}
	json.NewDecoder(resp.Body).Decode(&first)
	resp.Body.Close()

	// Wait until Run is actually entered before sending the second prompt.
	select {
	case <-agent.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("Run never entered")
	}

	// Second POST /v1/prompt while first is in flight — must steer, not re-run.
	body := `{"prompt":"second","session_id":"` + first.SessionID + `"}`
	r2, err := http.Post(ts.URL+"/v1/prompt", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if r2.StatusCode != http.StatusOK {
		t.Fatalf("second prompt while active: got %d, want 200", r2.StatusCode)
	}
	r2.Body.Close()

	// Release the blocking run so it can finish.
	close(release)

	// Wait for Run to finish so the deferred activeTurns clear runs.
	select {
	case <-agent.done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not finish after release")
	}

	if n := agent.runs.Load(); n != 1 {
		t.Fatalf("Run called %d times, want exactly 1 (second prompt must steer not re-run)", n)
	}
	steered := agent.Steered()
	if len(steered) != 1 || steered[0] != "second" {
		t.Fatalf("expected steer 'second', got %v", steered)
	}
}

// blockingSteerAgent parks in Run until release is closed.
// It uses atomic.Int32 for the run count and a plain []string guarded by mu
// for steered texts. The test goroutine reads both only after the done channel
// is closed, which is the formal happens-before boundary.
type blockingSteerAgent struct {
	release chan struct{}
	entered chan struct{}
	done    chan struct{}
	runs    atomic.Int32
	mu      sync.Mutex
	steered []string
}

func newBlockingSteerAgent(release chan struct{}) *blockingSteerAgent {
	return &blockingSteerAgent{
		release: release,
		entered: make(chan struct{}),
		done:    make(chan struct{}),
	}
}

func (a *blockingSteerAgent) Run(ctx context.Context, _ string, onEvent func(acp.AgentEvent)) error {
	a.runs.Add(1)
	close(a.entered)
	defer close(a.done)
	select {
	case <-a.release:
	case <-ctx.Done():
	}
	onEvent(acp.AgentEvent{Kind: acp.EventKindDone, StopReason: "end_turn"})
	return nil
}

func (a *blockingSteerAgent) Steer(text string) {
	a.mu.Lock()
	a.steered = append(a.steered, text)
	a.mu.Unlock()
}

// Steered returns a snapshot; call only after a.done is closed.
func (a *blockingSteerAgent) Steered() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]string, len(a.steered))
	copy(out, a.steered)
	return out
}
