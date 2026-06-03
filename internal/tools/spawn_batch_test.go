package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeBatchSpawner records how many concurrent spawns happen at peak.
type fakeBatchSpawner struct {
	active atomic.Int32
	peak   atomic.Int32
	delay  time.Duration
}

func (f *fakeBatchSpawner) Spawn(ctx context.Context, req SpawnRequest) (SpawnResult, error) {
	cur := f.active.Add(1)
	for {
		p := f.peak.Load()
		if cur <= p || f.peak.CompareAndSwap(p, cur) {
			break
		}
	}
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	f.active.Add(-1)
	return SpawnResult{Summary: fmt.Sprintf("done:%s", req.Description)}, nil
}

func TestSpawnBatchExecute(t *testing.T) {
	sp := &fakeBatchSpawner{delay: 10 * time.Millisecond}
	tool := NewSpawnBatchTool(sp, 3) // cap=3

	reqs := []map[string]any{
		{"description": "task-a"},
		{"description": "task-b"},
		{"description": "task-c"},
		{"description": "task-d"},
		{"description": "task-e"},
	}
	args, _ := json.Marshal(map[string]any{"tasks": reqs})

	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %s", res.Content)
	}

	// Results must appear in order: task[0] before task[1] before task[2] etc.
	lines := strings.Split(strings.TrimRight(res.Content, "\n"), "\n")
	if len(lines) != len(reqs) {
		t.Fatalf("expected %d output lines, got %d:\n%s", len(reqs), len(lines), res.Content)
	}
	for i, r := range reqs {
		wantLine := fmt.Sprintf("task[%d]: done:%s", i, r["description"])
		if lines[i] != wantLine {
			t.Errorf("line %d: got %q, want %q", i, lines[i], wantLine)
		}
	}

	// Peak concurrency must not exceed cap=3.
	if pk := sp.peak.Load(); pk > 3 {
		t.Errorf("peak concurrency = %d, want ≤ 3", pk)
	}
}

func TestSpawnBatchEmptyTasks(t *testing.T) {
	tool := NewSpawnBatchTool(nil, 4)
	args, _ := json.Marshal(map[string]any{"tasks": []any{}})
	res, _ := tool.Execute(context.Background(), args)
	if res.IsError {
		t.Fatalf("empty tasks should not error: %s", res.Content)
	}
}

func TestSpawnBatchNilSpawnerWithTasks(t *testing.T) {
	// spawner is nil but tasks is non-empty — should hit the nil-spawner error branch.
	tool := NewSpawnBatchTool(nil, 4)
	args, _ := json.Marshal(map[string]any{
		"tasks": []map[string]any{{"description": "task-a"}},
	})
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected error result when spawner is nil, got: %s", res.Content)
	}
	if !strings.Contains(res.Content, "no spawner configured") {
		t.Errorf("unexpected error message: %s", res.Content)
	}
}

func TestSpawnBatchMissingDescription(t *testing.T) {
	sp := &fakeBatchSpawner{}
	tool := NewSpawnBatchTool(sp, 2)
	// Task without description should produce a per-task error, not a top-level error.
	args, _ := json.Marshal(map[string]any{
		"tasks": []map[string]any{{"agent": "x"}}, // no description
	})
	res, _ := tool.Execute(context.Background(), args)
	// Tool should succeed at the outer level but report the individual failure.
	if res.IsError {
		t.Fatalf("outer error unexpected: %s", res.Content)
	}
}
