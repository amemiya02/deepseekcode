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

	// The reconciler must update in place: same ID, not a new record.
	if id1 != id2 {
		t.Errorf("expected update-in-place (same ID); got %q and %q — near-duplicate reconciliation is broken", id1, id2)
	}

	// Recall must return exactly one entry — the reconciler's core invariant.
	results, err := store.Recall("JSONL persistent memory")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("expected exactly 1 recalled result after update-in-place; got %d", len(results))
	}

	// The recalled record must carry the updated content.
	if len(results) == 1 {
		if results[0].Content != updated {
			t.Errorf("recalled content not updated: got %q, want %q", results[0].Content, updated)
		}

		// Tags must be merged: both "storage" and "disk" must be present.
		tagSet := make(map[string]bool, len(results[0].Tags))
		for _, tag := range results[0].Tags {
			tagSet[tag] = true
		}
		for _, want := range []string{"storage", "disk"} {
			if !tagSet[want] {
				t.Errorf("expected merged tag %q in recalled record; got tags %v", want, results[0].Tags)
			}
		}
	}
}

func TestReconcileDistinctFactsNotMerged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.jsonl")
	store, err := memory.NewJSONLStore(path)
	if err != nil {
		t.Fatal(err)
	}

	id1, err := store.Remember("The agent uses Go for backend work.", nil)
	if err != nil {
		t.Fatalf("first Remember: %v", err)
	}
	id2, err := store.Remember("The user prefers dark mode in VS Code.", nil)
	if err != nil {
		t.Fatalf("second Remember: %v", err)
	}

	if id1 == id2 {
		t.Error("completely unrelated facts must not be merged by reconciler")
	}

	results, err := store.Recall("Go backend agent")
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
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
