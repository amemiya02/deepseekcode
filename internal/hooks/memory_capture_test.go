package hooks_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/hooks"
	"github.com/amemiya02/deepseekcode/internal/memory"
)

func TestAutoCaptureSavesToStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.jsonl")
	store, err := memory.NewJSONLStore(path)
	if err != nil {
		t.Fatal(err)
	}

	capture := hooks.NewMemoryCapture(store)

	input := hooks.HookInput{
		Event:     hooks.EventPostToolUse,
		SessionID: "test-session-1",
		CWD:       "/tmp",
		Extra: map[string]any{
			"tool":   "read_file",
			"output": "User prefers stdlib over frameworks.",
		},
	}

	if err := capture.Handle(context.Background(), input); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	results, err := store.Recall("stdlib frameworks")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("auto-captured fact not found in store")
	}
}

func TestAutoCaptureDedupsOnRepeat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.jsonl")
	store, err := memory.NewJSONLStore(path)
	if err != nil {
		t.Fatal(err)
	}

	capture := hooks.NewMemoryCapture(store)
	input := hooks.HookInput{
		Event:     hooks.EventPostToolUse,
		SessionID: "test-session-2",
		CWD:       "/tmp",
		Extra: map[string]any{
			"tool":   "bash",
			"output": "Repeated tool output that should not be stored twice.",
		},
	}

	// Call twice — should not duplicate.
	_ = capture.Handle(context.Background(), input)
	_ = capture.Handle(context.Background(), input)

	results, _ := store.Recall("Repeated tool output")
	if len(results) > 1 {
		t.Errorf("expected at most 1 result after dedup; got %d", len(results))
	}
}

func TestAutoCaptureSessionEnd(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.jsonl")
	store, err := memory.NewJSONLStore(path)
	if err != nil {
		t.Fatal(err)
	}

	capture := hooks.NewMemoryCapture(store)
	input := hooks.HookInput{
		Event:     hooks.EventSessionEnd,
		SessionID: "test-session-end",
		CWD:       "/tmp",
		Extra: map[string]any{
			"summary": "Session completed refactoring internal/memory package.",
		},
	}

	if err := capture.Handle(context.Background(), input); err != nil {
		t.Fatalf("Handle SessionEnd: %v", err)
	}

	results, _ := store.Recall("refactoring memory package")
	if len(results) == 0 {
		t.Fatal("session-end summary not captured in store")
	}
}
