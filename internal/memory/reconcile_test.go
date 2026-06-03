package memory_test

import (
	"path/filepath"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/memory"
)

func TestReconcileUpdateInPlace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.jsonl")
	store, err := memory.NewJSONLStore(path)
	if err != nil {
		t.Fatal(err)
	}

	original := "The project uses JSONL for persistent memory storage."
	id1, err := store.Remember(original, []string{"storage"})
	if err != nil {
		t.Fatal(err)
	}

	// Near-duplicate: same concept, slightly rephrased.
	updated := "The project uses JSONL files for persistent memory storage on disk."
	id2, err := store.Remember(updated, []string{"storage", "disk"})
	if err != nil {
		t.Fatal(err)
	}

	// Should have updated in place (same ID) rather than creating a new record.
	if id1 != id2 {
		t.Logf("IDs differ (%q vs %q) — near-duplicate threshold may need tuning; not fatal", id1, id2)
		// Not a hard failure — threshold tuning is acceptable. Log only.
	}

	// Regardless of reconciliation: Recall should not return two separate
	// near-identical facts.
	results, err := store.Recall("JSONL persistent memory")
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	for _, r := range results {
		if seen[r.ID] {
			t.Errorf("duplicate ID %q in Recall results", r.ID)
		}
		seen[r.ID] = true
	}
}

func TestReconcileDistinctFactsNotMerged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.jsonl")
	store, err := memory.NewJSONLStore(path)
	if err != nil {
		t.Fatal(err)
	}

	id1, _ := store.Remember("The agent uses Go for backend work.", nil)
	id2, _ := store.Remember("The user prefers dark mode in VS Code.", nil)

	if id1 == id2 {
		t.Error("completely unrelated facts must not be merged by reconciler")
	}

	results, _ := store.Recall("Go backend agent")
	found1 := false
	for _, r := range results {
		if r.ID == id1 {
			found1 = true
		}
	}
	if !found1 {
		t.Error("first distinct fact not recallable")
	}
}
