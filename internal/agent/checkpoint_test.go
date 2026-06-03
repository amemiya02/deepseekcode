package agent

import (
	"context"
	"sync"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
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

func TestAgentBranchAtByStep(t *testing.T) {
	a := &Agent{checkpoints: newCheckpointIndex()}
	// Three steps: boundaries at messages 2, 4, 6.
	a.steps = []StepRecord{
		{MessageCount: 2, Snapshotted: true},
		{MessageCount: 4, Snapshotted: true},
		{MessageCount: 6, Snapshotted: true},
	}
	a.Messages = make([]llm.Message, 6)

	// Branch at step 1 (0-indexed) → stepIdx=1, messageCount=4.
	stepIdx, boundary, err := a.resolveBranchBoundary("1")
	if err != nil {
		t.Fatalf("resolveBranchBoundary: %v", err)
	}
	if stepIdx != 1 {
		t.Fatalf("stepIdx = %d, want 1", stepIdx)
	}
	if boundary != 4 {
		t.Fatalf("boundary = %d, want 4", boundary)
	}
}

func TestAgentBranchAtByName(t *testing.T) {
	a := &Agent{checkpoints: newCheckpointIndex()}
	a.steps = []StepRecord{
		{MessageCount: 3, Snapshotted: true},
		{MessageCount: 7, Snapshotted: true},
	}
	a.Messages = make([]llm.Message, 7)
	a.checkpoints.Record("pre-test", 1) // step index 1 → MessageCount 7

	stepIdx, boundary, err := a.resolveBranchBoundary("pre-test")
	if err != nil {
		t.Fatalf("resolveBranchBoundary by name: %v", err)
	}
	if stepIdx != 1 {
		t.Fatalf("stepIdx = %d, want 1", stepIdx)
	}
	if boundary != 7 {
		t.Fatalf("boundary = %d, want 7", boundary)
	}
}

// TestResolveBranchBoundaryErrors covers all error paths.
func TestResolveBranchBoundaryErrors(t *testing.T) {
	t.Run("empty steps slice", func(t *testing.T) {
		a := &Agent{checkpoints: newCheckpointIndex()}
		// a.steps is nil/empty
		_, _, err := a.resolveBranchBoundary("0")
		if err == nil {
			t.Fatal("expected error for empty steps, got nil")
		}
	})

	t.Run("step out of range negative", func(t *testing.T) {
		a := &Agent{checkpoints: newCheckpointIndex()}
		a.steps = []StepRecord{{MessageCount: 2}}
		_, _, err := a.resolveBranchBoundary("-1")
		if err == nil {
			t.Fatal("expected error for step -1, got nil")
		}
	})

	t.Run("step out of range too large", func(t *testing.T) {
		a := &Agent{checkpoints: newCheckpointIndex()}
		a.steps = []StepRecord{{MessageCount: 2}}
		_, _, err := a.resolveBranchBoundary("5")
		if err == nil {
			t.Fatal("expected error for step 5 with 1 step, got nil")
		}
	})

	t.Run("unknown checkpoint name", func(t *testing.T) {
		a := &Agent{checkpoints: newCheckpointIndex()}
		a.steps = []StepRecord{{MessageCount: 2}}
		_, _, err := a.resolveBranchBoundary("no-such-checkpoint")
		if err == nil {
			t.Fatal("expected error for unknown checkpoint, got nil")
		}
	})

	t.Run("step below compactionFloor", func(t *testing.T) {
		a := &Agent{checkpoints: newCheckpointIndex()}
		a.steps = []StepRecord{
			{MessageCount: 2},
			{MessageCount: 4},
			{MessageCount: 6},
		}
		// Simulate a compaction at step 2: steps 0 and 1 are stale.
		a.compactionFloor = 2
		_, _, err := a.resolveBranchBoundary("1")
		if err == nil {
			t.Fatal("expected error when step index is below compactionFloor, got nil")
		}
	})

	t.Run("checkpoint below compactionFloor", func(t *testing.T) {
		a := &Agent{checkpoints: newCheckpointIndex()}
		a.steps = []StepRecord{
			{MessageCount: 2},
			{MessageCount: 4},
			{MessageCount: 6},
		}
		a.checkpoints.Record("stale", 0) // step 0 is below compactionFloor=1
		a.compactionFloor = 1
		_, _, err := a.resolveBranchBoundary("stale")
		if err == nil {
			t.Fatal("expected error when checkpoint step is below compactionFloor, got nil")
		}
	})
}

// TestBranchAtNilWorktree exercises BranchAt end-to-end with wt=nil
// (dry-run mode), verifying StepIdx and MessageCount are set correctly.
func TestBranchAtNilWorktree(t *testing.T) {
	a := &Agent{checkpoints: newCheckpointIndex()}
	a.steps = []StepRecord{
		{MessageCount: 5, Snapshotted: true},
		{MessageCount: 10, Snapshotted: true},
	}
	a.Messages = make([]llm.Message, 10)
	a.checkpoints.Record("snap", 1) // step 1 → MessageCount 10

	t.Run("by step index", func(t *testing.T) {
		res, err := a.BranchAt(context.Background(), "0", nil)
		if err != nil {
			t.Fatalf("BranchAt: %v", err)
		}
		if res.StepIdx != 0 {
			t.Fatalf("StepIdx = %d, want 0", res.StepIdx)
		}
		if res.MessageCount != 5 {
			t.Fatalf("MessageCount = %d, want 5", res.MessageCount)
		}
		if res.WorktreePath != "" || res.Branch != "" {
			t.Fatalf("expected empty WorktreePath/Branch for nil wt, got %+v", res)
		}
	})

	t.Run("by checkpoint name", func(t *testing.T) {
		res, err := a.BranchAt(context.Background(), "snap", nil)
		if err != nil {
			t.Fatalf("BranchAt by name: %v", err)
		}
		if res.StepIdx != 1 {
			t.Fatalf("StepIdx = %d, want 1", res.StepIdx)
		}
		if res.MessageCount != 10 {
			t.Fatalf("MessageCount = %d, want 10", res.MessageCount)
		}
	})
}

// TestBranchAtCompactionFloor ensures BranchAt propagates the compactionFloor
// error from resolveBranchBoundary rather than silently returning stale data.
func TestBranchAtCompactionFloor(t *testing.T) {
	a := &Agent{checkpoints: newCheckpointIndex()}
	a.steps = []StepRecord{
		{MessageCount: 3},
		{MessageCount: 8},
		{MessageCount: 12},
	}
	a.compactionFloor = 2 // steps 0 and 1 are stale

	_, err := a.BranchAt(context.Background(), "1", nil)
	if err == nil {
		t.Fatal("BranchAt should have returned an error for a step below compactionFloor")
	}
}

// TestSanitizeBranchName verifies that '/' is replaced and safe chars pass through.
func TestSanitizeBranchName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"hello", "hello"},
		{"step-1", "step-1"},
		{"pre_test", "pre_test"},
		{"feature/foo", "feature-foo"}, // '/' must be replaced
		{"my branch!", "my-branch-"},   // space and '!' replaced
		{"ABC123", "ABC123"},
	}
	for _, tc := range cases {
		got := sanitizeBranchName(tc.in)
		if got != tc.want {
			t.Errorf("sanitizeBranchName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
