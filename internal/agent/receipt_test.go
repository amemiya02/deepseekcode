package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/session"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

func TestAgentWritesModelFinalReceipt(t *testing.T) {
	// Set up test server that returns a model response with usage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		emitSSE(w,
			`{"choices":[{"index":0,"delta":{"content":"ok"}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":50,"prompt_cache_hit_tokens":10,"prompt_cache_miss_tokens":90,"total_tokens":150}}`,
		)
	}))
	defer srv.Close()

	// Set up session store
	dir := t.TempDir()
	store, err := session.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	sess, err := store.NewSession(ctx, "/proj", "deepseek-v4-flash", false)
	if err != nil {
		t.Fatal(err)
	}

	client := llm.NewClient("k", srv.URL)
	reg := tools.New()
	pol := permissions.New(permissions.ModeYolo, "", nil, nil, nil)
	a := New(client, reg, pol, "deepseek-v4-flash")
	a.System = "sys"
	a.StopWhen = []StopCondition{MaxSteps(1)}
	a.Persister = session.NewPersister(store, nil, sess.ID)

	_, err = a.Run(ctx, "hello")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Verify model_final receipt was written
	receipts, err := store.ListReceipts(ctx, sess.ID)
	if err != nil {
		t.Fatalf("ListReceipts: %v", err)
	}

	var modelReceipt *session.TranscriptReceipt
	for i := range receipts {
		if receipts[i].Kind == session.ReceiptModelFinal {
			modelReceipt = &receipts[i]
			break
		}
	}

	if modelReceipt == nil {
		t.Fatal("expected model_final receipt, got none")
	}

	// Verify payload contains required fields
	var payload map[string]any
	if err := json.Unmarshal(modelReceipt.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if _, ok := payload["model"]; !ok {
		t.Error("payload missing 'model' field")
	}
	if _, ok := payload["usage"]; !ok {
		t.Error("payload missing 'usage' field")
	}
	if _, ok := payload["prefix_hash"]; !ok {
		t.Error("payload missing 'prefix_hash' field")
	}
	if _, ok := payload["prefix_system_hash"]; !ok {
		t.Error("payload missing 'prefix_system_hash' field")
	}
	if _, ok := payload["prefix_tools_hash"]; !ok {
		t.Error("payload missing 'prefix_tools_hash' field")
	}

	// Verify prefix_hash is non-empty
	if hash, ok := payload["prefix_hash"].(string); !ok || hash == "" {
		t.Error("prefix_hash should be non-empty string")
	}
}

func TestAgentWritesRepairReceipt(t *testing.T) {
	// Set up test server that returns a response triggering repair
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Return a response with invalid JSON in tool args to trigger repair
		emitSSE(w,
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"echo","arguments":"{invalid"}}]}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`,
		)
	}))
	defer srv.Close()

	dir := t.TempDir()
	store, err := session.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	sess, err := store.NewSession(ctx, "/proj", "test-model", false)
	if err != nil {
		t.Fatal(err)
	}

	client := llm.NewClient("k", srv.URL)
	reg := tools.New()
	reg.Register(echoTool{})
	pol := permissions.New(permissions.ModeYolo, "", nil, nil, nil)
	a := New(client, reg, pol, "test-model")
	a.System = "sys"
	a.StopWhen = []StopCondition{MaxSteps(2)}
	a.Persister = session.NewPersister(store, nil, sess.ID)

	_, err = a.Run(ctx, "hello")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Verify repair receipt was written
	receipts, err := store.ListReceipts(ctx, sess.ID)
	if err != nil {
		t.Fatalf("ListReceipts: %v", err)
	}

	var repairReceipt *session.TranscriptReceipt
	for i := range receipts {
		if receipts[i].Kind == session.ReceiptRepair {
			repairReceipt = &receipts[i]
			break
		}
	}

	if repairReceipt == nil {
		t.Fatal("expected repair receipt, got none")
	}

	// Verify payload contains repair fields
	var payload map[string]any
	if err := json.Unmarshal(repairReceipt.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if _, ok := payload["Kind"]; !ok {
		t.Error("payload missing 'Kind' field")
	}
	if _, ok := payload["Tool"]; !ok {
		t.Error("payload missing 'Tool' field")
	}
}

func TestAgentWritesPermissionReceipt(t *testing.T) {
	// Set up test server that returns a tool call (so permission gate is exercised)
	var stepCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stepCount++
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if stepCount == 1 {
			// Return a tool call to trigger permission gate
			emitSSE(w,
				`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"echo","arguments":"{}"}}]}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`,
			)
		} else {
			// Second step: return stop
			emitSSE(w,
				`{"choices":[{"index":0,"delta":{"content":"done"}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`,
			)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	store, err := session.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	sess, err := store.NewSession(ctx, "/proj", "test-model", false)
	if err != nil {
		t.Fatal(err)
	}

	client := llm.NewClient("k", srv.URL)
	reg := tools.New()
	reg.Register(echoTool{})
	pol := permissions.New(permissions.ModeYolo, "", nil, nil, nil)
	a := New(client, reg, pol, "test-model")
	a.System = "sys"
	a.StopWhen = []StopCondition{MaxSteps(2)}
	a.Persister = session.NewPersister(store, nil, sess.ID)

	_, err = a.Run(ctx, "hello")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Verify permission receipt was written (Yolo mode auto-allows)
	receipts, err := store.ListReceipts(ctx, sess.ID)
	if err != nil {
		t.Fatalf("ListReceipts: %v", err)
	}

	var permReceipt *session.TranscriptReceipt
	for i := range receipts {
		if receipts[i].Kind == session.ReceiptPermission {
			permReceipt = &receipts[i]
			break
		}
	}

	if permReceipt == nil {
		t.Fatal("expected permission receipt, got none")
	}

	// Verify payload contains permission fields
	var payload map[string]any
	if err := json.Unmarshal(permReceipt.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if _, ok := payload["tool"]; !ok {
		t.Error("payload missing 'tool' field")
	}
	if _, ok := payload["decision"]; !ok {
		t.Error("payload missing 'decision' field")
	}

	// In Yolo mode, decision should be "allow"
	if decision, ok := payload["decision"].(string); !ok || decision != "allow" {
		t.Errorf("expected decision 'allow', got %v", payload["decision"])
	}
}
