package tools_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/memory"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

func makeTestStore(t *testing.T) *memory.JSONLStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "memory.jsonl")
	store, err := memory.NewJSONLStore(path)
	if err != nil {
		t.Fatalf("NewJSONLStore: %v", err)
	}
	return store
}

func TestRememberToolExecute(t *testing.T) {
	store := makeTestStore(t)
	tool := tools.NewRememberTool(store)

	if tool.Name() == "" {
		t.Error("RememberTool.Name() must be non-empty")
	}
	if tool.Description() == "" {
		t.Error("RememberTool.Description() must be non-empty")
	}

	args, _ := json.Marshal(map[string]any{
		"content": "Remember: user prefers short answers.",
		"tags":    []string{"preference"},
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Content == "" {
		t.Fatal("Execute returned empty result content")
	}
}

func TestRecallToolExecute(t *testing.T) {
	store := makeTestStore(t)
	// Pre-populate
	if _, err := store.Remember("User works in Go.", nil); err != nil {
		t.Fatal(err)
	}

	tool := tools.NewRecallTool(store)
	args, _ := json.Marshal(map[string]any{
		"query": "Go",
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Content == "" {
		t.Fatal("Execute returned empty result content")
	}
	// Must contain the stored content, not just a non-empty (e.g. "[]") string.
	if !strings.Contains(result.Content, "User works in Go.") {
		t.Fatalf("recall result does not contain expected content; got: %s", result.Content)
	}
}

func TestForgetToolExecute(t *testing.T) {
	store := makeTestStore(t)
	id, err := store.Remember("Temporary fact.", nil)
	if err != nil {
		t.Fatal(err)
	}

	tool := tools.NewForgetTool(store)
	args, _ := json.Marshal(map[string]any{
		"id": id,
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Content == "" {
		t.Fatal("Execute returned empty result content")
	}

	// Verify forgotten
	results, _ := store.Recall("Temporary fact")
	if len(results) != 0 {
		t.Errorf("fact not forgotten; still in store: %v", results)
	}
}

func TestToolsRegistration(t *testing.T) {
	store := makeTestStore(t)
	reg := tools.New()
	tools.RegisterMemoryTools(reg, store)

	for _, name := range []string{"remember", "recall", "forget"} {
		if _, ok := reg.Get(name); !ok {
			t.Errorf("tool %q not found in registry after RegisterMemoryTools", name)
		}
	}
}

// ---------------------------------------------------------------------------
// Validation-branch coverage
// ---------------------------------------------------------------------------

func TestRememberToolValidation(t *testing.T) {
	store := makeTestStore(t)
	tool := tools.NewRememberTool(store)

	t.Run("invalid JSON", func(t *testing.T) {
		result, err := tool.Execute(context.Background(), json.RawMessage(`{bad`))
		if err != nil {
			t.Fatalf("unexpected hard error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result for invalid JSON")
		}
	})

	t.Run("empty content", func(t *testing.T) {
		args, _ := json.Marshal(map[string]any{"content": ""})
		result, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("unexpected hard error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result for empty content")
		}
	})
}

func TestRecallToolValidation(t *testing.T) {
	store := makeTestStore(t)
	tool := tools.NewRecallTool(store)

	t.Run("invalid JSON", func(t *testing.T) {
		result, err := tool.Execute(context.Background(), json.RawMessage(`{bad`))
		if err != nil {
			t.Fatalf("unexpected hard error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result for invalid JSON")
		}
	})

	t.Run("empty query", func(t *testing.T) {
		args, _ := json.Marshal(map[string]any{"query": ""})
		result, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("unexpected hard error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result for empty query")
		}
	})
}

func TestForgetToolValidation(t *testing.T) {
	store := makeTestStore(t)
	tool := tools.NewForgetTool(store)

	t.Run("invalid JSON", func(t *testing.T) {
		result, err := tool.Execute(context.Background(), json.RawMessage(`{bad`))
		if err != nil {
			t.Fatalf("unexpected hard error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result for invalid JSON")
		}
	})

	t.Run("empty id", func(t *testing.T) {
		args, _ := json.Marshal(map[string]any{"id": ""})
		result, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("unexpected hard error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result for empty id")
		}
	})

	t.Run("not-found id", func(t *testing.T) {
		args, _ := json.Marshal(map[string]any{"id": "does-not-exist"})
		result, err := tool.Execute(context.Background(), args)
		// Must be a tool-result error (err==nil), NOT a hard infrastructure error.
		if err != nil {
			t.Fatalf("not-found should produce tool result, not hard error; got err: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result for not-found id")
		}
		if !strings.Contains(result.Content, "not found") {
			t.Fatalf("error message should mention 'not found'; got: %s", result.Content)
		}
	})
}
