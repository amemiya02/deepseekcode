package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type fakeSpawner struct {
	result SpawnResult
	err    error
	called bool
	req    SpawnRequest
}

func (f *fakeSpawner) Spawn(_ context.Context, req SpawnRequest) (SpawnResult, error) {
	f.called = true
	f.req = req
	return f.result, f.err
}

func TestSubagentToolBasics(t *testing.T) {
	tool := NewSubagentTool(&fakeSpawner{})
	if tool.Name() != "task" {
		t.Fatalf("Name() = %q, want %q", tool.Name(), "task")
	}
	if tool.IsReadOnly() {
		t.Fatal("IsReadOnly() should be false")
	}
}

func TestSubagentToolExecute(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		sp := &fakeSpawner{result: SpawnResult{Summary: "done", StepCount: 2, TokenCount: 100}}
		tool := NewSubagentTool(sp)
		res, err := tool.Execute(context.Background(), json.RawMessage(`{"description":"find bugs"}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.IsError {
			t.Fatalf("expected success, got error: %s", res.Content)
		}
		if res.Content != "done" {
			t.Errorf("Content = %q, want %q", res.Content, "done")
		}
		if !sp.called {
			t.Error("Spawn not called")
		}
		if sp.req.Description != "find bugs" {
			t.Errorf("Description = %q, want %q", sp.req.Description, "find bugs")
		}
	})

	t.Run("with agent and tools", func(t *testing.T) {
		sp := &fakeSpawner{result: SpawnResult{Summary: "ok"}}
		tool := NewSubagentTool(sp)
		res, err := tool.Execute(context.Background(), json.RawMessage(`{"agent":"explore","description":"x","tools":["grep"]}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error result: %s", res.Content)
		}
		if sp.req.Agent != "explore" {
			t.Errorf("Agent = %q, want %q", sp.req.Agent, "explore")
		}
		if len(sp.req.Tools) != 1 || sp.req.Tools[0] != "grep" {
			t.Errorf("Tools = %v, want [grep]", sp.req.Tools)
		}
	})

	t.Run("missing description", func(t *testing.T) {
		tool := NewSubagentTool(&fakeSpawner{})
		res, err := tool.Execute(context.Background(), json.RawMessage(`{"agent":"explore"}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for missing description")
		}
	})

	t.Run("empty description", func(t *testing.T) {
		tool := NewSubagentTool(&fakeSpawner{})
		res, err := tool.Execute(context.Background(), json.RawMessage(`{"description":""}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for empty description")
		}
	})

	t.Run("nil spawner", func(t *testing.T) {
		tool := NewSubagentTool(nil)
		res, err := tool.Execute(context.Background(), json.RawMessage(`{"description":"x"}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for nil spawner")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		tool := NewSubagentTool(&fakeSpawner{})
		res, err := tool.Execute(context.Background(), json.RawMessage(`{bad json`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for invalid json")
		}
	})

	t.Run("spawn error becomes result error", func(t *testing.T) {
		sp := &fakeSpawner{err: errors.New("spawn failed")}
		tool := NewSubagentTool(sp)
		res, err := tool.Execute(context.Background(), json.RawMessage(`{"description":"x"}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for spawn error")
		}
	})

	t.Run("empty summary gets placeholder", func(t *testing.T) {
		sp := &fakeSpawner{result: SpawnResult{Summary: ""}}
		tool := NewSubagentTool(sp)
		res, err := tool.Execute(context.Background(), json.RawMessage(`{"description":"x"}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Content != "(subagent returned no summary)" {
			t.Errorf("Content = %q, want placeholder", res.Content)
		}
	})
}
