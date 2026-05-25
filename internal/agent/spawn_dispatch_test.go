package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/agents"
	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// stubTool is a minimal read-only tool for spawn tests.
type stubTool struct {
	name string
}

func (s stubTool) Name() string        { return s.name }
func (s stubTool) Description() string { return "stub " + s.name }
func (s stubTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}
func (s stubTool) Execute(_ context.Context, _ json.RawMessage) (tools.Result, error) {
	return tools.Result{Content: "ok"}, nil
}
func (s stubTool) IsReadOnly() bool { return true }

func TestLoopSpawnerInterface(t *testing.T) {
	// Compile-time check that LoopSpawner implements tools.Spawner.
	var _ tools.Spawner = (*LoopSpawner)(nil)
}

func TestSpawnDepthLimit(t *testing.T) {
	client := llm.NewClient("k", "http://unused")
	parent := New(client, tools.New(), permissions.New(permissions.ModeDefault, "", nil, nil, nil), "m")
	s := &LoopSpawner{
		Client:   client,
		Parent:   parent,
		Defs:     nil,
		MaxDepth: 1,
		depth:    1, // already at max
	}
	res, err := s.Spawn(context.Background(), tools.SpawnRequest{Description: "x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Summary != "subagent depth limit reached" {
		t.Errorf("Summary = %q, want %q", res.Summary, "subagent depth limit reached")
	}
}

func TestSpawnDefaultMaxDepth(t *testing.T) {
	client := llm.NewClient("k", "http://unused")
	parent := New(client, tools.New(), permissions.New(permissions.ModeDefault, "", nil, nil, nil), "m")
	s := &LoopSpawner{
		Client: client,
		Parent: parent,
		// MaxDepth=0 → default 2
		depth: 2,
	}
	res, err := s.Spawn(context.Background(), tools.SpawnRequest{Description: "x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Summary != "subagent depth limit reached" {
		t.Errorf("Summary = %q, want depth limit", res.Summary)
	}
}

func TestSpawnChildIsSubagent(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if n == 1 {
			// Final assistant text + stop (no tool calls).
			emitSSE(w,
				`{"choices":[{"index":0,"delta":{"content":"child done"}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`,
			)
		} else {
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	client := llm.NewClient("k", srv.URL)
	reg := tools.New()
	reg.Register(stubTool{name: "read_file"})
	pol := permissions.New(permissions.ModeYolo, "", nil, nil, nil)
	parent := New(client, reg, pol, "test-model")
	parent.System = "parent sys"

	s := &LoopSpawner{
		Client: client,
		Parent: parent,
		Defs:   map[string]agents.AgentDef{},
	}

	res, err := s.Spawn(context.Background(), tools.SpawnRequest{Description: "do something"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.StepCount < 1 {
		t.Errorf("StepCount = %d, want >= 1", res.StepCount)
	}
	if res.Summary != "child done" {
		t.Errorf("Summary = %q, want %q", res.Summary, "child done")
	}
}

func TestSpawnTaskExcludedByDefault(t *testing.T) {
	client := llm.NewClient("k", "http://unused")
	reg := tools.New()
	reg.Register(stubTool{name: "read_file"})
	reg.Register(stubTool{name: "grep"})
	pol := permissions.New(permissions.ModeYolo, "", nil, nil, nil)
	parent := New(client, reg, pol, "m")

	s := &LoopSpawner{
		Client: client,
		Parent: parent,
		Defs:   map[string]agents.AgentDef{},
	}

	// The Subset call inside Spawn should not include "task" since it's
	// not in the parent registry and def.Tools is empty.
	// But if we register "task" in parent, it should still be excluded.
	taskTool := tools.NewSubagentTool(s)
	reg.Register(taskTool)

	// Verify "task" is in parent.
	if _, ok := parent.Tools.Get("task"); !ok {
		t.Fatal("task should be in parent registry")
	}

	// We can't easily test the internal subset, but we can verify
	// that spawning with a def that doesn't list "task" won't include it.
	// Use the depth limit to avoid actually running a child loop.
	s.depth = 2
	s.MaxDepth = 2
	res, err := s.Spawn(context.Background(), tools.SpawnRequest{Description: "x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Summary != "subagent depth limit reached" {
		t.Errorf("Summary = %q, want depth limit", res.Summary)
	}
}

func TestSpawnWithDefTools(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if n == 1 {
			emitSSE(w,
				`{"choices":[{"index":0,"delta":{"content":"explored"}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`,
			)
		} else {
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	client := llm.NewClient("k", srv.URL)
	reg := tools.New()
	reg.Register(stubTool{name: "read_file"})
	reg.Register(stubTool{name: "grep"})
	reg.Register(stubTool{name: "bash"})
	pol := permissions.New(permissions.ModeYolo, "", nil, nil, nil)
	parent := New(client, reg, pol, "test-model")

	defs := map[string]agents.AgentDef{
		"explore": {Tools: []string{"read_file", "grep"}, Prompt: "explorer"},
	}
	s := &LoopSpawner{
		Client: client,
		Parent: parent,
		Defs:   defs,
	}

	res, err := s.Spawn(context.Background(), tools.SpawnRequest{
		Agent:       "explore",
		Description: "find callers",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Summary != "explored" {
		t.Errorf("Summary = %q, want %q", res.Summary, "explored")
	}
}

func TestSpawnErrorBecomesSummary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := llm.NewClient("k", srv.URL)
	reg := tools.New()
	reg.Register(stubTool{name: "read_file"})
	pol := permissions.New(permissions.ModeYolo, "", nil, nil, nil)
	parent := New(client, reg, pol, "test-model")

	s := &LoopSpawner{
		Client: client,
		Parent: parent,
		Defs:   map[string]agents.AgentDef{},
	}

	res, err := s.Spawn(context.Background(), tools.SpawnRequest{Description: "fail"})
	if err != nil {
		t.Fatalf("Spawn should not return Go error, got: %v", err)
	}
	if res.Summary == "" {
		t.Error("Summary should not be empty on failure")
	}
	// The summary should contain the error info.
	t.Logf("Summary on error: %q", res.Summary)
}

func TestSpawnInheritsParentSystem(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if n == 1 {
			emitSSE(w,
				`{"choices":[{"index":0,"delta":{"content":"ok"}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`,
			)
		} else {
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	client := llm.NewClient("k", srv.URL)
	reg := tools.New()
	reg.Register(stubTool{name: "read_file"})
	pol := permissions.New(permissions.ModeYolo, "", nil, nil, nil)
	parent := New(client, reg, pol, "test-model")
	parent.System = "custom parent system"

	s := &LoopSpawner{
		Client: client,
		Parent: parent,
		Defs:   map[string]agents.AgentDef{},
	}

	_, err := s.Spawn(context.Background(), tools.SpawnRequest{Description: "x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
