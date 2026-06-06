package tools

import (
	"context"
	"encoding/json"
	"testing"
)

type fakeCheckpointRecorder struct {
	recorded map[string]int
	step     int
}

func (f *fakeCheckpointRecorder) RecordCheckpoint(name string) int {
	if f.recorded == nil {
		f.recorded = make(map[string]int)
	}
	f.recorded[name] = f.step
	step := f.step
	f.step++
	return step
}

func TestCheckpointToolExecute(t *testing.T) {
	rec := &fakeCheckpointRecorder{step: 5}
	tool := NewCheckpointTool(rec)

	args, _ := json.Marshal(map[string]any{"name": "v1"})
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", res.Content)
	}
	if rec.recorded["v1"] != 5 {
		t.Fatalf("recorded step = %d, want 5", rec.recorded["v1"])
	}
}

func TestCheckpointToolMissingName(t *testing.T) {
	rec := &fakeCheckpointRecorder{step: 2}
	tool := NewCheckpointTool(rec)

	args, _ := json.Marshal(map[string]any{})
	res, _ := tool.Execute(context.Background(), args)
	if !res.IsError {
		t.Fatal("expected error for missing name")
	}
}

func TestCheckpointToolIsReadOnly(t *testing.T) {
	tool := NewCheckpointTool(nil)
	if tool.IsReadOnly() {
		t.Fatal("checkpoint tool must not be read-only: RecordCheckpoint mutates agent state")
	}
}
