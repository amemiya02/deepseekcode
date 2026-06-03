package memory_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/memory"
)

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.jsonl")

	store, err := memory.NewJSONLStore(path)
	if err != nil {
		t.Fatalf("NewJSONLStore: %v", err)
	}

	id, err := store.Remember("The agent prefers terse replies.", []string{"preference", "style"})
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty ID")
	}

	results, err := store.Recall("terse replies")
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result, got none")
	}
	if results[0].Content != "The agent prefers terse replies." {
		t.Fatalf("unexpected content: %q", results[0].Content)
	}

	if err := store.Forget(id); err != nil {
		t.Fatalf("Forget: %v", err)
	}

	results2, err := store.Recall("terse replies")
	if err != nil {
		t.Fatalf("Recall after Forget: %v", err)
	}
	if len(results2) != 0 {
		t.Fatalf("expected no results after Forget, got %d", len(results2))
	}

	_ = os.Remove(path)
}

func TestStoreMultipleRecall(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.jsonl")

	store, err := memory.NewJSONLStore(path)
	if err != nil {
		t.Fatalf("NewJSONLStore: %v", err)
	}

	facts := []string{
		"User works in Go and prefers stdlib.",
		"Project uses JSONL for storage.",
		"Cache prefix must stay frozen.",
	}
	for _, f := range facts {
		if _, err := store.Remember(f, nil); err != nil {
			t.Fatalf("Remember %q: %v", f, err)
		}
	}

	results, err := store.Recall("JSONL storage")
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results for JSONL storage query")
	}
	// Top result should mention JSONL
	found := false
	for _, r := range results {
		if r.Content == "Project uses JSONL for storage." {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("top results did not contain JSONL fact; got: %v", results)
	}
}

func TestStorePersistsAcrossReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.jsonl")

	store1, _ := memory.NewJSONLStore(path)
	if _, err := store1.Remember("Persisted fact across reload.", []string{"reload"}); err != nil {
		t.Fatal(err)
	}

	store2, err := memory.NewJSONLStore(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	results, err := store2.Recall("Persisted fact")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("fact not found after store reload")
	}
}
