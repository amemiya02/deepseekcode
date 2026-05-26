package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// fakeJobController implements JobController for testing
type fakeJobController struct {
	startErr      error
	jobID         string
	startCalled   bool
	lastCommand   string
	lastUsePTY    bool
	lastTimeoutMs int
}

func (f *fakeJobController) StartBashJob(ctx context.Context, command string, usePTY bool, timeoutMs int) (string, error) {
	f.startCalled = true
	f.lastCommand = command
	f.lastUsePTY = usePTY
	f.lastTimeoutMs = timeoutMs
	if f.startErr != nil {
		return "", f.startErr
	}
	if f.jobID == "" {
		return "job-123", nil
	}
	return f.jobID, nil
}

func TestBackgroundBashTool_Name(t *testing.T) {
	tool := NewBackgroundBashTool(nil)
	if tool.Name() != "background_bash" {
		t.Errorf("expected name 'background_bash', got %q", tool.Name())
	}
}

func TestBackgroundBashTool_Parameters(t *testing.T) {
	tool := NewBackgroundBashTool(nil)
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
	if _, ok := props["command"]; !ok {
		t.Error("expected 'command' property")
	}
	if _, ok := props["pty"]; !ok {
		t.Error("expected 'pty' property")
	}
	if _, ok := props["timeout_ms"]; !ok {
		t.Error("expected 'timeout_ms' property")
	}
}

func TestBackgroundBashTool_IsReadOnly(t *testing.T) {
	tool := NewBackgroundBashTool(nil)
	if tool.IsReadOnly() {
		t.Error("background_bash should not be read-only")
	}
}

func TestBackgroundBashTool_NilController(t *testing.T) {
	tool := NewBackgroundBashTool(nil)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"echo hello"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content == "" {
		t.Error("expected error message in content")
	}
}

func TestBackgroundBashTool_MissingCommand(t *testing.T) {
	ctrl := &fakeJobController{}
	tool := NewBackgroundBashTool(ctrl)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content == "" {
		t.Error("expected error message for missing command")
	}
}

func TestBackgroundBashTool_EmptyCommand(t *testing.T) {
	ctrl := &fakeJobController{}
	tool := NewBackgroundBashTool(ctrl)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"command":""}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content == "" {
		t.Error("expected error message for empty command")
	}
}

func TestBackgroundBashTool_DefaultTimeout(t *testing.T) {
	ctrl := &fakeJobController{}
	tool := NewBackgroundBashTool(ctrl)

	_, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"echo hello"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !ctrl.startCalled {
		t.Error("expected StartBashJob to be called")
	}
	if ctrl.lastCommand != "echo hello" {
		t.Errorf("expected command 'echo hello', got %q", ctrl.lastCommand)
	}
	if ctrl.lastTimeoutMs != 600000 {
		t.Errorf("expected default timeout 600000, got %d", ctrl.lastTimeoutMs)
	}
}

func TestBackgroundBashTool_CustomTimeout(t *testing.T) {
	ctrl := &fakeJobController{}
	tool := NewBackgroundBashTool(ctrl)

	_, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"sleep 10","timeout_ms":30000}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ctrl.lastTimeoutMs != 30000 {
		t.Errorf("expected timeout 30000, got %d", ctrl.lastTimeoutMs)
	}
}

func TestBackgroundBashTool_UsePTY(t *testing.T) {
	ctrl := &fakeJobController{}
	tool := NewBackgroundBashTool(ctrl)

	_, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"vim","pty":true}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !ctrl.lastUsePTY {
		t.Error("expected usePTY to be true")
	}
}

func TestBackgroundBashTool_Success(t *testing.T) {
	ctrl := &fakeJobController{jobID: "test-job-456"}
	tool := NewBackgroundBashTool(ctrl)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"echo test"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Content == "" {
		t.Error("expected content with job ID")
	}
	if !strings.Contains(result.Content, "started job test-job-456") {
		t.Errorf("expected content to contain 'started job test-job-456', got %q", result.Content)
	}
	if !strings.Contains(result.Content, "task_status") {
		t.Errorf("expected content to mention 'task_status', got %q", result.Content)
	}
}
