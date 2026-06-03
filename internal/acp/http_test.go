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

func TestHTTPGatewayCreateSession(t *testing.T) {
	ts, _ := setupGateway(t)
	resp, err := http.Post(ts.URL+"/session", "application/json",
		strings.NewReader(`{"workingDir":"/tmp"}`))
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
	ts, _ := setupGateway(t)

	// 1. Create session.
	resp1, err := http.Post(ts.URL+"/session", "application/json",
		strings.NewReader(`{"workingDir":"/tmp"}`))
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
		http.Post(ts.URL+"/session/"+newRes.SessionID+"/prompt", "application/json", //nolint
			strings.NewReader(`{"prompt":"world"}`))
	}()

	// 4. Read SSE lines until we see a done event or timeout.
	scanner := bufio.NewScanner(streamResp.Body)
	var sawData bool
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data:") {
			sawData = true
		}
		if strings.Contains(line, "done") {
			break
		}
	}
	if !sawData {
		t.Error("expected at least one SSE data line")
	}
}

func TestHTTPGatewayDeleteSession(t *testing.T) {
	ts, _ := setupGateway(t)

	// Create session.
	resp1, err := http.Post(ts.URL+"/session", "application/json",
		strings.NewReader(`{"workingDir":"/tmp"}`))
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
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on delete, got %d", resp2.StatusCode)
	}
	resp2.Body.Close()

	// A subsequent prompt to the deleted session must return 404.
	resp3, err := http.Post(ts.URL+"/session/"+newRes.SessionID+"/prompt", "application/json",
		strings.NewReader(`{"prompt":"after-delete"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp3.Body.Close()
	if resp3.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", resp3.StatusCode)
	}
}

func TestHTTPGatewayMissingSession(t *testing.T) {
	ts, _ := setupGateway(t)
	resp, _ := http.Post(ts.URL+"/session/nonexistent/prompt", "application/json",
		strings.NewReader(`{"prompt":"hi"}`))
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}
