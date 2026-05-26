package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeJobStatusController implements JobStatusController for testing
type fakeJobStatusController struct {
	status        Status
	statusErr     error
	canceled      bool
	cancelErr     error
	lastTailLines int
}

func (f *fakeJobStatusController) JobStatus(id string, tailLines int) (Status, error) {
	f.lastTailLines = tailLines
	return f.status, f.statusErr
}

func (f *fakeJobStatusController) CancelJob(id string) error {
	f.canceled = true
	return f.cancelErr
}

func TestTaskStatusTool_Name(t *testing.T) {
	tool := NewTaskStatusTool(nil)
	if tool.Name() != "task_status" {
		t.Errorf("expected name 'task_status', got %q", tool.Name())
	}
}

func TestTaskStatusTool_IsReadOnly(t *testing.T) {
	tool := NewTaskStatusTool(nil)
	if !tool.IsReadOnly() {
		t.Error("task_status should be read-only")
	}
}

func TestTaskStatusTool_Parameters(t *testing.T) {
	tool := NewTaskStatusTool(nil)
	params := tool.Parameters()

	var schema map[string]any
	if err := json.Unmarshal(params, &schema); err != nil {
		t.Fatalf("failed to parse parameters: %v", err)
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties in schema")
	}

	// Check required properties
	if _, ok := props["job_id"]; !ok {
		t.Error("expected 'job_id' property")
	}
	if _, ok := props["tail_lines"]; !ok {
		t.Error("expected 'tail_lines' property")
	}
	if _, ok := props["cancel"]; !ok {
		t.Error("expected 'cancel' property")
	}
}

func TestTaskStatusTool_NilController(t *testing.T) {
	tool := NewTaskStatusTool(nil)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"job_id":"job-123"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content == "" {
		t.Error("expected error message in content")
	}
}

func TestTaskStatusTool_MissingJobID(t *testing.T) {
	ctrl := &fakeJobStatusController{}
	tool := NewTaskStatusTool(ctrl)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content == "" {
		t.Error("expected error message for missing job_id")
	}
}

func TestTaskStatusTool_EmptyJobID(t *testing.T) {
	ctrl := &fakeJobStatusController{}
	tool := NewTaskStatusTool(ctrl)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"job_id":""}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content == "" {
		t.Error("expected error message for empty job_id")
	}
}

func TestTaskStatusTool_JobNotFound(t *testing.T) {
	ctrl := &fakeJobStatusController{
		statusErr: errors.New("job not found"),
	}
	tool := NewTaskStatusTool(ctrl)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"job_id":"nonexistent"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content == "" {
		t.Error("expected error message for unknown job")
	}
}

func TestTaskStatusTool_Success(t *testing.T) {
	ctrl := &fakeJobStatusController{
		status: Status{
			ID:        "job-123",
			Kind:      "background_bash",
			State:     "succeeded",
			StartedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			Summary:   "exit 0",
			Tail:      "ok",
		},
	}
	tool := NewTaskStatusTool(ctrl)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"job_id":"job-123"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Content == "" {
		t.Error("expected content with job status")
	}
	if !strings.Contains(result.Content, "state: succeeded") {
		t.Errorf("expected 'state: succeeded' in content, got %q", result.Content)
	}
	if !strings.Contains(result.Content, "ok") {
		t.Errorf("expected 'ok' in content, got %q", result.Content)
	}
	if ctrl.canceled {
		t.Error("should not have called CancelJob")
	}
}

func TestTaskStatusTool_WithOutput(t *testing.T) {
	ctrl := &fakeJobStatusController{
		status: Status{
			ID:        "job-456",
			Kind:      "subagent",
			State:     "running",
			StartedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			Tail:      "Processing...\nStill working...",
		},
	}
	tool := NewTaskStatusTool(ctrl)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"job_id":"job-456","tail_lines":50}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Content == "" {
		t.Error("expected content with output")
	}
	if ctrl.lastTailLines != 50 {
		t.Errorf("expected tail_lines=50 to reach controller, got %d", ctrl.lastTailLines)
	}
}

func TestTaskStatusTool_TruncatedOutput(t *testing.T) {
	ctrl := &fakeJobStatusController{
		status: Status{
			ID:           "job-789",
			Kind:         "background_bash",
			State:        "succeeded",
			StartedAt:    time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			Tail:         "last line",
			DroppedBytes: 1000,
			Truncated:    true,
		},
	}
	tool := NewTaskStatusTool(ctrl)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"job_id":"job-789"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Content == "" {
		t.Error("expected content with truncation note")
	}
}

func TestTaskStatusTool_Cancel(t *testing.T) {
	ctrl := &fakeJobStatusController{
		status: Status{
			ID:        "job-999",
			Kind:      "background_bash",
			State:     "running",
			StartedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		},
	}
	tool := NewTaskStatusTool(ctrl)

	_, err := tool.Execute(context.Background(), json.RawMessage(`{"job_id":"job-999","cancel":true}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !ctrl.canceled {
		t.Error("expected CancelJob to be called")
	}
}

func TestTaskStatusTool_EmptyOutput(t *testing.T) {
	ctrl := &fakeJobStatusController{
		status: Status{
			ID:        "job-empty",
			Kind:      "background_bash",
			State:     "succeeded",
			StartedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			Summary:   "exit 0",
			Tail:      "",
		},
	}
	tool := NewTaskStatusTool(ctrl)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"job_id":"job-empty"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Content == "" {
		t.Error("expected content")
	}
}

func TestTaskStatusTool_FinishedTimestamp(t *testing.T) {
	ctrl := &fakeJobStatusController{
		status: Status{
			ID:         "job-finished",
			Kind:       "background_bash",
			State:      "failed",
			StartedAt:  time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			FinishedAt: time.Date(2024, 1, 1, 12, 1, 0, 0, time.UTC),
			Summary:    "exit 1",
		},
	}
	tool := NewTaskStatusTool(ctrl)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"job_id":"job-finished"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Content == "" {
		t.Error("expected content with finished timestamp")
	}
}
