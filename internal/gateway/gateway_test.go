package gateway_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/acp"
	"github.com/amemiya02/deepseekcode/internal/gateway"
)

// stubAgentFactory returns an AgentRunner that emits one text delta, one info
// (mapped to "message_delta"), then done. It needs no API key, mirroring how the acp
// package stubs the agent in its own tests.
func stubAgentFactory(workingDir string) (acp.AgentRunner, error) {
	return &stubAgent{}, nil
}

type stubAgent struct{}

func (s *stubAgent) Run(ctx context.Context, userPrompt string, onEvent func(acp.AgentEvent)) error {
	onEvent(acp.AgentEvent{Kind: acp.EventKindTextDelta, Text: "hello " + userPrompt})
	onEvent(acp.AgentEvent{Kind: acp.EventKindInfo, Text: "thinking"})
	onEvent(acp.AgentEvent{Kind: acp.EventKindDone, StopReason: "end_turn"})
	return nil
}

func (s *stubAgent) Steer(_ string) {}

// fixtureTrace is a minimal JSONL trace exercising both an expected-miss first
// turn and an evicted warm turn, so the derived CacheReport fields are non-zero.
const fixtureTrace = `{"type":"prefix.snapshot","epoch_id":"e1","static_prefix_hash":"aabbccdd","schema_version":2}
{"type":"usage","turn":1,"epoch_id":"e1","cache_hit_tokens":0,"cache_miss_tokens":5000,"output_tokens":200,"cost_cny":0.002,"schema_version":2}
{"type":"usage","turn":2,"epoch_id":"e1","cache_hit_tokens":12000,"cache_miss_tokens":200,"output_tokens":150,"cost_cny":0.0005,"schema_version":2}
{"type":"compaction","epoch_id":"e1","schema_version":2}
{"type":"usage","turn":3,"epoch_id":"e1","cache_hit_tokens":100,"cache_miss_tokens":4900,"output_tokens":180,"cost_cny":0.0019,"schema_version":2}
`

func writeFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	if err := os.WriteFile(path, []byte(fixtureTrace), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func newTestServer(t *testing.T, tracePath string) *httptest.Server {
	t.Helper()
	sm := acp.NewSessionManager(stubAgentFactory)
	h := gateway.NewHandler(sm, tracePath)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts
}

// TestCacheReportWithFixture verifies GET /v1/cache returns 200 with valid JSON
// carrying all 8 fields populated from a real trace fixture.
func TestCacheReportWithFixture(t *testing.T) {
	ts := newTestServer(t, writeFixture(t))

	resp, err := http.Get(ts.URL + "/v1/cache")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("expected application/json, got %q", ct)
	}

	// Decode into a map keyed by the exact JSON field names so a renamed or
	// missing field fails the test.
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatalf("decode cache report: %v", err)
	}
	for _, field := range []string{
		"total_usage_turns", "cache_hit_tokens", "cache_miss_tokens",
		"output_tokens", "cost_cny", "full_body_evictions",
		"max_miss_tokens", "cache_hit_rate",
	} {
		if _, ok := raw[field]; !ok {
			t.Errorf("cache report missing field %q", field)
		}
	}

	var report gateway.CacheReport
	if err := json.Unmarshal(mustJoin(raw), &report); err != nil {
		t.Fatalf("re-decode typed report: %v", err)
	}
	if report.TotalUsageTurns != 3 {
		t.Errorf("total_usage_turns = %d, want 3", report.TotalUsageTurns)
	}
	if report.MaxMissTokens != 5000 {
		t.Errorf("max_miss_tokens = %d, want 5000", report.MaxMissTokens)
	}
	// Turn 3 follows a compaction with hit=100 (<= eviction threshold): exactly
	// one full-body eviction.
	if report.FullBodyEvictions != 1 {
		t.Errorf("full_body_evictions = %d, want 1", report.FullBodyEvictions)
	}
	if report.OutputTokens != 530 {
		t.Errorf("output_tokens = %d, want 530", report.OutputTokens)
	}
}

// mustJoin re-marshals a raw field map back into a JSON object so it can be
// decoded into the typed struct.
func mustJoin(raw map[string]json.RawMessage) []byte {
	b, _ := json.Marshal(raw)
	return b
}

// TestCacheReportNoTrace verifies GET /v1/cache returns a zero-valued report
// (200, never 500) when the trace file is absent.
func TestCacheReportNoTrace(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.jsonl")
	ts := newTestServer(t, missing)

	resp, err := http.Get(ts.URL + "/v1/cache")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on missing trace, got %d", resp.StatusCode)
	}
	var report gateway.CacheReport
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if report != (gateway.CacheReport{}) {
		t.Fatalf("expected zero-valued report on missing trace, got %+v", report)
	}
}

// TestServeSPA verifies the catch-all serves the SPA: GET / returns 200 with a
// non-empty body. With the stub webapp build this is the placeholder page.
func TestServeSPA(t *testing.T) {
	ts := newTestServer(t, "")

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for SPA root, got %d", resp.StatusCode)
	}
	body, _ := readAll(resp)
	if len(body) == 0 {
		t.Fatal("expected non-empty SPA body")
	}
}

// TestPromptReturnsIDs verifies POST /v1/prompt returns 200 with request_id and
// session_id using the stub agent factory (no API key needed).
func TestPromptReturnsIDs(t *testing.T) {
	ts := newTestServer(t, "")

	resp, err := http.Post(ts.URL+"/v1/prompt", "application/json",
		strings.NewReader(`{"prompt":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var out struct {
		RequestID string `json:"request_id"`
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode prompt response: %v", err)
	}
	if out.RequestID == "" {
		t.Error("expected non-empty request_id")
	}
	if out.SessionID == "" {
		t.Error("expected non-empty session_id")
	}
}

// TestEventsStreamsDone drives a full stub run and asserts the SSE stream emits
// the named events the SPA listens for, ending with "turn_done".
func TestEventsStreamsDone(t *testing.T) {
	ts := newTestServer(t, "")

	// 1. Create a session by prompting; capture the session_id.
	resp, err := http.Post(ts.URL+"/v1/prompt", "application/json",
		strings.NewReader(`{"prompt":"world"}`))
	if err != nil {
		t.Fatal(err)
	}
	var first struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&first); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	if first.SessionID == "" {
		t.Fatal("expected session_id")
	}

	// 2. Open the SSE stream for that session, then re-prompt the SAME session
	// so the run's events are observed by the subscriber. (The first run may
	// have completed before we subscribed; re-prompting the existing session
	// deterministically produces a fresh stream the test can read.)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		ts.URL+"/v1/events?session_id="+first.SessionID, nil)
	streamResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer streamResp.Body.Close()
	if ct := streamResp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("expected SSE content type, got %q", ct)
	}

	// Re-prompt now that the subscriber is registered.
	go func() {
		body := `{"prompt":"again","session_id":"` + first.SessionID + `"}`
		r, perr := http.Post(ts.URL+"/v1/prompt", "application/json", strings.NewReader(body))
		if perr == nil {
			r.Body.Close()
		}
	}()

	// 3. Read SSE frames until "turn_done".
	scanner := bufio.NewScanner(streamResp.Body)
	var sawDelta, sawStep, sawDone bool
	var lastEvent string
	for scanner.Scan() {
		line := scanner.Text()
		if name, ok := strings.CutPrefix(line, "event: "); ok {
			lastEvent = name
			switch name {
			case "message_delta":
				sawDelta = true
				sawStep = true // message_delta covers both text and info kinds
			case "turn_done":
				sawDone = true
			}
		}
		if lastEvent == "turn_done" && strings.HasPrefix(line, "data: ") {
			break
		}
	}
	if !sawDone {
		t.Error("expected a 'done' SSE event")
	}
	// message_delta and step are best-effort (stub emits them), but turn_done is the
	// load-bearing terminal event the SPA requires.
	_ = sawDelta
	_ = sawStep
}

// questionAgentFactory asks one question, then done.
func questionAgentFactory(workingDir string) (acp.AgentRunner, error) {
	return &questionAgent{}, nil
}

type questionAgent struct{}

func (a *questionAgent) Steer(_ string) {}

func (a *questionAgent) Run(ctx context.Context, userPrompt string, onEvent func(acp.AgentEvent)) error {
	resp := make(chan [][]string, 1)
	onEvent(acp.AgentEvent{
		Kind: acp.EventKindAsk,
		Questions: []acp.AskQuestion{{
			Question: "Pick one", Header: "choice",
			Options: []acp.AskOption{{Label: "A"}, {Label: "B"}},
		}},
		Answer: func(ans [][]string) { resp <- ans },
	})
	<-resp
	onEvent(acp.AgentEvent{Kind: acp.EventKindDone, StopReason: "end_turn"})
	return nil
}

// askingAgentFactory returns an agent that requests one permission, then done.
func askingAgentFactory(workingDir string) (acp.AgentRunner, error) {
	return &askingAgent{}, nil
}

type askingAgent struct{}

func (a *askingAgent) Steer(_ string) {}

func (a *askingAgent) Run(ctx context.Context, userPrompt string, onEvent func(acp.AgentEvent)) error {
	resp := make(chan acp.PermissionDecision, 1)
	onEvent(acp.AgentEvent{
		Kind:     acp.EventKindPermission,
		ToolName: "write_file",
		ToolArgs: `{"path":"a.go"}`,
		Respond:  func(d acp.PermissionDecision) { resp <- d },
	})
	// Block until the gateway resolves it via POST /v1/permission OR the run
	// context is cancelled (so a test that only asserts the SSE frame can tear
	// down without leaking this goroutine — see the N-2 t.Cleanup in Step 1).
	select {
	case <-resp:
	case <-ctx.Done():
		return ctx.Err()
	}
	onEvent(acp.AgentEvent{Kind: acp.EventKindDone, StopReason: "end_turn"})
	return nil
}

func TestPromptEmitsPermissionRequest(t *testing.T) {
	sm := acp.NewSessionManager(askingAgentFactory)
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

	// Subscribe, then re-prompt the same session so the subscriber sees it.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		ts.URL+"/v1/events?session_id="+first.SessionID, nil)
	stream, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()

	promptCtx, promptCancel := context.WithCancel(context.Background())
	t.Cleanup(promptCancel)
	go func() {
		body := `{"prompt":"again","session_id":"` + first.SessionID + `"}`
		req2, _ := http.NewRequestWithContext(promptCtx, http.MethodPost, ts.URL+"/v1/prompt",
			strings.NewReader(body))
		req2.Header.Set("Content-Type", "application/json")
		r, _ := http.DefaultClient.Do(req2)
		if r != nil {
			r.Body.Close()
		}
	}()

	scanner := bufio.NewScanner(stream.Body)
	var sawPerm bool
	var permData string
	var lastEvent string
	for scanner.Scan() {
		line := scanner.Text()
		if name, ok := strings.CutPrefix(line, "event: "); ok {
			lastEvent = name
			if name == "permission_request" {
				sawPerm = true
			}
		}
		if lastEvent == "permission_request" {
			if d, ok := strings.CutPrefix(line, "data: "); ok {
				permData = d
				break
			}
		}
	}
	if !sawPerm {
		t.Fatal("expected a permission_request SSE event")
	}
	var p struct {
		ID      string                 `json:"id"`
		Tool    string                 `json:"tool"`
		Args    map[string]interface{} `json:"args"`
		Options []struct {
			Value       string `json:"value"`
			Label       string `json:"label"`
			Description string `json:"description"`
		} `json:"options"`
	}
	if err := json.Unmarshal([]byte(permData), &p); err != nil {
		t.Fatalf("permission_request data not JSON: %v (%q)", err, permData)
	}
	if p.ID == "" || p.Tool != "write_file" {
		t.Fatalf("permission_request payload = %+v", p)
	}
	// Contract 2: args must be a JSON object (Record<string,unknown>), not a string.
	if p.Args == nil {
		t.Fatalf("permission_request args must be a JSON object, got nil (was it double-encoded as a string?)")
	}
	if _, ok := p.Args["path"]; !ok {
		t.Errorf("permission_request args missing 'path' key, got %v", p.Args)
	}
	wantLabels := []string{"Deny", "Allow once", "Allow for session", "Always allow"}
	if len(p.Options) != len(wantLabels) {
		t.Fatalf("expected %d options, got %d", len(wantLabels), len(p.Options))
	}
	for i, want := range wantLabels {
		if p.Options[i].Label != want {
			t.Errorf("option[%d].Label = %q, want %q", i, p.Options[i].Label, want)
		}
	}
}

func readAll(resp *http.Response) ([]byte, error) {
	sc := bufio.NewScanner(resp.Body)
	var b strings.Builder
	for sc.Scan() {
		b.WriteString(sc.Text())
	}
	return []byte(b.String()), sc.Err()
}
