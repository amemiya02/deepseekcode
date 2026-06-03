package tools_test

import (
	"context"
	"encoding/json"
	"path/filepath"
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
	reg.Register(tools.NewRememberTool(store))
	reg.Register(tools.NewRecallTool(store))
	reg.Register(tools.NewForgetTool(store))

	for _, name := range []string{"remember", "recall", "forget"} {
		if _, ok := reg.Get(name); !ok {
			t.Errorf("tool %q not found in registry after Register", name)
		}
	}
}
