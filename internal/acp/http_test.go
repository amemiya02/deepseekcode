package acp_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/acp"
)

func setupGateway(t *testing.T) (*httptest.Server, *acp.HTTPGateway) {
	t.Helper()
	sm := acp.NewSessionManager(stubAgentFactory)
	gw := acp.NewHTTPGateway(sm)
	ts := httptest.NewServer(gw)
	t.Cleanup(ts.Close)
	return ts, gw
}

// authPost issues a POST carrying the gateway bearer token. The gateway rejects
// any request lacking a valid Authorization header with 401, so all
// happy-path tests authenticate through this helper.
func authPost(t *testing.T, gw *acp.HTTPGateway, url, body string) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+gw.Token())
	return http.DefaultClient.Do(req)
}

func TestHTTPGatewayCreateSession(t *testing.T) {
	ts, gw := setupGateway(t)
	resp, err := authPost(t, gw, ts.URL+"/session", `{"workingDir":"/tmp"}`)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var result acp.SessionNewResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode session result: %v", err)
	}
	if result.SessionID == "" {
		t.Fatal("expected sessionId")
	}
}

func TestHTTPGatewaySSEStream(t *testing.T) {
	ts, gw := setupGateway(t)

	// 1. Create session.
	resp1, err := authPost(t, gw, ts.URL+"/session", `{"workingDir":"/tmp"}`)
	if err != nil {
		t.Fatal(err)
	}
	var newRes acp.SessionNewResult
	if err := json.NewDecoder(resp1.Body).Decode(&newRes); err != nil {
		t.Fatalf("decode new-session result: %v", err)
	}
	resp1.Body.Close()
	if newRes.SessionID == "" {
		t.Fatal("expected non-empty sessionId from session creation")
	}

	// 2. Open SSE stream.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		ts.URL+"/session/"+newRes.SessionID+"/stream", nil)
	req.Header.Set("Authorization", "Bearer "+gw.Token())
	streamResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer streamResp.Body.Close()
	if ct := streamResp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("expected SSE content type, got %q", ct)
	}

	// 3. Send prompt. The gateway's handlePrompt blocks on readyCh until the
	// SSE client registered above signals readiness, so no artificial sleep
	// is needed here — the synchronisation is handled by the gateway itself.
	go func() {
		authPost(t, gw, ts.URL+"/session/"+newRes.SessionID+"/prompt", `{"prompt":"world"}`) //nolint
	}()

	// 4. Read SSE lines until we see a completion event (non-empty stopReason)
	// or timeout. We unmarshal each "data:" frame rather than substring-matching
	// the literal "done": the stub emits a DoneParams with stopReason="end_turn"
	// and no "done" token, so the old break condition never fired and the test
	// passed only by exhausting the 5s deadline. Parsing stopReason makes the
	// test detect completion and return promptly.
	scanner := bufio.NewScanner(streamResp.Body)
	var sawData, sawDone bool
	for scanner.Scan() {
		line := scanner.Text()
		payload, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		sawData = true
		var frame struct {
			StopReason string `json:"stopReason"`
		}
		if err := json.Unmarshal([]byte(payload), &frame); err != nil {
			t.Fatalf("unmarshal SSE data frame %q: %v", payload, err)
		}
		if frame.StopReason != "" {
			sawDone = true
			break
		}
	}
	if !sawData {
		t.Error("expected at least one SSE data line")
	}
	if !sawDone {
		t.Error("expected an SSE frame carrying a non-empty stopReason (stream completion)")
	}
}

func TestHTTPGatewayDeleteSession(t *testing.T) {
	ts, gw := setupGateway(t)

	// Create session.
	resp1, err := authPost(t, gw, ts.URL+"/session", `{"workingDir":"/tmp"}`)
	if err != nil {
		t.Fatal(err)
	}
	var newRes acp.SessionNewResult
	if err := json.NewDecoder(resp1.Body).Decode(&newRes); err != nil {
		t.Fatalf("decode new-session result: %v", err)
	}
	resp1.Body.Close()
	if newRes.SessionID == "" {
		t.Fatal("expected non-empty sessionId")
	}

	// Delete the session.
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/session/"+newRes.SessionID, nil)
	req.Header.Set("Authorization", "Bearer "+gw.Token())
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on delete, got %d", resp2.StatusCode)
	}
	resp2.Body.Close()

	// A subsequent prompt to the deleted session must return 404.
	resp3, err := authPost(t, gw, ts.URL+"/session/"+newRes.SessionID+"/prompt", `{"prompt":"after-delete"}`)
	if err != nil {
		t.Fatal(err)
	}
	resp3.Body.Close()
	if resp3.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", resp3.StatusCode)
	}
}

func TestHTTPGatewayMissingSession(t *testing.T) {
	ts, gw := setupGateway(t)
	resp, err := authPost(t, gw, ts.URL+"/session/nonexistent/prompt", `{"prompt":"hi"}`)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// TestHTTPGatewayRejectsMissingToken verifies that a request without the bearer
// token is rejected with 401 before any session work happens. This is the core
// defense against an unauthenticated network host driving the agent.
func TestHTTPGatewayRejectsMissingToken(t *testing.T) {
	ts, _ := setupGateway(t)
	// No Authorization header at all.
	resp, err := http.Post(ts.URL+"/session", "application/json",
		strings.NewReader(`{"workingDir":"/tmp"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without bearer token, got %d", resp.StatusCode)
	}
}

// TestHTTPGatewayRejectsWrongToken verifies that a request presenting an
// incorrect bearer token is rejected with 401.
func TestHTTPGatewayRejectsWrongToken(t *testing.T) {
	ts, _ := setupGateway(t)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/session",
		strings.NewReader(`{"workingDir":"/tmp"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer not-the-real-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 with wrong bearer token, got %d", resp.StatusCode)
	}
}

// TestHTTPGatewayAcceptsValidToken verifies the happy path: a request carrying
// the correct bearer token is processed normally (200 with a session id).
func TestHTTPGatewayAcceptsValidToken(t *testing.T) {
	ts, gw := setupGateway(t)
	resp, err := authPost(t, gw, ts.URL+"/session", `{"workingDir":"/tmp"}`)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with valid bearer token, got %d", resp.StatusCode)
	}
	var result acp.SessionNewResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode session result: %v", err)
	}
	if result.SessionID == "" {
		t.Fatal("expected sessionId with valid bearer token")
	}
}
