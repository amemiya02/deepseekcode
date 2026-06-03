package agent

import "testing"

func TestCheckpointIndex(t *testing.T) {
	idx := newCheckpointIndex()

	// Record a checkpoint.
	idx.Record("alpha", 3)
	si, ok := idx.Lookup("alpha")
	if !ok || si != 3 {
		t.Fatalf("Lookup(alpha) = (%d, %v), want (3, true)", si, ok)
	}

	// Overwrite is allowed (idempotent re-checkpoint at later step).
	idx.Record("alpha", 7)
	si, ok = idx.Lookup("alpha")
	if !ok || si != 7 {
		t.Fatalf("overwrite Lookup = (%d, %v), want (7, true)", si, ok)
	}

	// Missing name.
	_, ok = idx.Lookup("nonexistent")
	if ok {
		t.Fatalf("Lookup(nonexistent) should be false")
	}

	// List returns recorded names.
	idx.Record("beta", 9)
	names := idx.Names()
	if len(names) != 2 {
		t.Fatalf("Names() = %v, want 2 entries", names)
	}
}
