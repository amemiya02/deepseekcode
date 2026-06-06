package memory_test

import (
	"path/filepath"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/memory"
)

func TestSHADedupPreventsDuplicates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.jsonl")
	store, err := memory.NewJSONLStore(path)
	if err != nil {
		t.Fatal(err)
	}

	fact := "SHA dedup prevents exact duplicates."
	id1, err := store.Remember(fact, nil)
	if err != nil {
		t.Fatalf("first Remember: %v", err)
	}
	id2, err := store.Remember(fact, nil)
	if err != nil {
		t.Fatalf("second Remember: %v", err)
	}
	if id1 != id2 {
		t.Errorf("expected same ID for duplicate fact; got %q and %q", id1, id2)
	}

	// Recall should return exactly one result.
	results, err := store.Recall("SHA dedup")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestContentSHADeterministic(t *testing.T) {
	h1 := memory.ContentSHA("hello world")
	h2 := memory.ContentSHA("hello world")
	if h1 != h2 {
		t.Errorf("SHA not deterministic: %q vs %q", h1, h2)
	}
	h3 := memory.ContentSHA("hello world!")
	if h1 == h3 {
		t.Error("different inputs produced same SHA")
	}
}
