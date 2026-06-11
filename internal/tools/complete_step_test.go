package tools

import (
	"context"
	"testing"
)

type fakeStepRecorder struct {
	steps []StepEvidence
}

func (f *fakeStepRecorder) RecordStep(ev StepEvidence) {
	f.steps = append(f.steps, ev)
}

func TestCompleteStepRecordsEvidence(t *testing.T) {
	rec := &fakeStepRecorder{}
	tool := CompleteStep{Controller: rec}
	if tool.Name() != "complete_step" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "complete_step")
	}
	if !tool.IsReadOnly() {
		t.Error("IsReadOnly() should be true")
	}
	args := mustJSON(t, map[string]any{"step": "run tests", "evidence": "all 42 passed"})
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %s", res.Content)
	}
	if len(rec.steps) != 1 {
		t.Fatalf("RecordStep called %d times, want 1", len(rec.steps))
	}
	if rec.steps[0].Step != "run tests" {
		t.Errorf("step = %q, want %q", rec.steps[0].Step, "run tests")
	}
	if rec.steps[0].Evidence != "all 42 passed" {
		t.Errorf("evidence = %q, want %q", rec.steps[0].Evidence, "all 42 passed")
	}
}

func TestCompleteStepRequiresEvidence(t *testing.T) {
	rec := &fakeStepRecorder{}
	tool := CompleteStep{Controller: rec}
	args := mustJSON(t, map[string]any{"step": "run tests"})
	res, _ := tool.Execute(context.Background(), args)
	if !res.IsError {
		t.Fatal("expected error for missing evidence")
	}
	if len(rec.steps) != 0 {
		t.Error("RecordStep should not be called on validation failure")
	}
}

func TestCompleteStepRequiresStep(t *testing.T) {
	rec := &fakeStepRecorder{}
	tool := CompleteStep{Controller: rec}
	args := mustJSON(t, map[string]any{"evidence": "all passed"})
	res, _ := tool.Execute(context.Background(), args)
	if !res.IsError {
		t.Fatal("expected error for missing step")
	}
	if len(rec.steps) != 0 {
		t.Error("RecordStep should not be called on validation failure")
	}
}
