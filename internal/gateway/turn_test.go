package gateway_test

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
