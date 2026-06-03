package agent

import (
	"sync"
	"testing"
)

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

	// Names() returns recorded names in sorted order.
	idx.Record("beta", 9)
	names := idx.Names()
	if len(names) != 2 {
		t.Fatalf("Names() = %v, want 2 entries", names)
	}
	if names[0] != "alpha" || names[1] != "beta" {
		t.Fatalf("Names() = %v, want [alpha beta] (sorted)", names)
	}
}

func TestAgentRecordCheckpoint(t *testing.T) {
	// Agent.RecordCheckpoint increments steps then records.
	// Use a minimal agent with no LLM — just test the bookkeeping.
	a := &Agent{checkpoints: newCheckpointIndex()}
	// Simulate two steps already taken.
	a.steps = []StepRecord{{}, {}}
	step := a.RecordCheckpoint("mid")
	if step != 2 {
		t.Fatalf("RecordCheckpoint step = %d, want 2", step)
	}
	si, ok := a.checkpoints.Lookup("mid")
	if !ok || si != 2 {
		t.Fatalf("Lookup = (%d, %v), want (2, true)", si, ok)
	}
}

// TestCheckpointIndexConcurrent exercises concurrent Record and Lookup under
// the race detector to validate the RWMutex contract.
func TestCheckpointIndexConcurrent(t *testing.T) {
	idx := newCheckpointIndex()
	const goroutines = 20
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Writers.
	for g := 0; g < goroutines; g++ {
		g := g
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				idx.Record("writer", g*iterations+i)
			}
		}()
	}

	// Readers.
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				idx.Lookup("writer")
				idx.Names()
			}
		}()
	}

	wg.Wait()
}
