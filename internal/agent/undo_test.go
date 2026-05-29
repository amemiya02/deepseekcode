package agent

import (
	"context"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// fakePersister implements Persister + MessageTruncator, recording the keep
// count passed to TruncateMessages.
type fakePersister struct {
	truncCalls  int
	truncatedTo int
	diskCount   int // reported by PersistedMessageCount (alignment check)
}

func (f *fakePersister) SessionID() string { return "s" }
func (f *fakePersister) AppendUserMessage(context.Context, []llm.ContentBlock) (int, error) {
	return 0, nil
}
func (f *fakePersister) AppendAssistant(context.Context, []llm.ContentBlock, string, llm.Usage) (int, error) {
	return 0, nil
}
func (f *fakePersister) AppendToolResult(context.Context, string, string, bool) (int, error) {
	return 0, nil
}
func (f *fakePersister) TakeSnapshot(int, []string) (int, error)      { return 0, nil }
func (f *fakePersister) SetActiveModel(context.Context, string) error { return nil }
func (f *fakePersister) ReplaceWithCompaction(context.Context, int, int, string) (int, error) {
	return 0, nil
}
func (f *fakePersister) TruncateMessages(_ context.Context, keep int) (int, error) {
	f.truncCalls++
	f.truncatedTo = keep
	return 0, nil
}
func (f *fakePersister) PersistedMessageCount(context.Context) (int, error) {
	return f.diskCount, nil
}

// msgs builds n placeholder body messages.
func msgs(n int) []llm.Message {
	out := make([]llm.Message, n)
	for i := range out {
		out[i] = llm.Message{Role: "user"}
	}
	return out
}

// newUndoAgent builds an agent with a fixed transcript and step history. Steps
// carry (MessageCount, Snapshotted); a.Messages has msgLen placeholder msgs.
func newUndoAgent(steps []StepRecord, msgLen int, p Persister) *Agent {
	a := New(nil, nil, nil, "m")
	a.System = "SYSTEM-PROMPT"
	a.Persister = p
	a.steps = steps
	a.Messages = msgs(msgLen)
	return a
}

func TestReconcileUndo(t *testing.T) {
	// Steps: idx0 start@1, idx1 snapshot@3, idx2 start@5, idx3 snapshot@7.
	// Transcript length 9.
	mkSteps := func() []StepRecord {
		return []StepRecord{
			{MessageCount: 1},
			{MessageCount: 3, Snapshotted: true},
			{MessageCount: 5},
			{MessageCount: 7, Snapshotted: true},
		}
	}

	t.Run("undo last snapshot truncates to its boundary", func(t *testing.T) {
		fp := &fakePersister{diskCount: 9} // aligned with the 9 in-memory msgs
		a := newUndoAgent(mkSteps(), 9, fp)
		preSys := a.staticSystem()

		removed, err := a.ReconcileUndo(context.Background(), 1)
		if err != nil {
			t.Fatalf("ReconcileUndo: %v", err)
		}
		if removed != 2 { // 9 - 7
			t.Errorf("removed = %d, want 2", removed)
		}
		if len(a.Messages) != 7 {
			t.Errorf("len(Messages) = %d, want 7", len(a.Messages))
		}
		if len(a.steps) != 3 { // dropped step idx3
			t.Errorf("len(steps) = %d, want 3", len(a.steps))
		}
		if fp.truncCalls != 1 || fp.truncatedTo != 7 {
			t.Errorf("persister truncate = (%d calls, keep %d), want (1, 7)", fp.truncCalls, fp.truncatedTo)
		}
		if a.staticSystem() != preSys {
			t.Error("static prefix (system) changed across undo; fingerprint must be byte-identical")
		}
	})

	t.Run("undo two snapshots reaches the earlier boundary", func(t *testing.T) {
		a := newUndoAgent(mkSteps(), 9, nil)
		removed, err := a.ReconcileUndo(context.Background(), 2)
		if err != nil {
			t.Fatalf("ReconcileUndo: %v", err)
		}
		if len(a.Messages) != 3 || len(a.steps) != 1 || removed != 6 {
			t.Errorf("got len(Messages)=%d len(steps)=%d removed=%d, want 3/1/6", len(a.Messages), len(a.steps), removed)
		}
	})

	t.Run("n exceeding snapshots clamps to the earliest snapshot", func(t *testing.T) {
		a := newUndoAgent(mkSteps(), 9, nil)
		if _, err := a.ReconcileUndo(context.Background(), 99); err != nil {
			t.Fatalf("ReconcileUndo: %v", err)
		}
		// earliest snapshot is idx1 @ boundary 3
		if len(a.Messages) != 3 || len(a.steps) != 1 {
			t.Errorf("got len(Messages)=%d len(steps)=%d, want 3/1", len(a.Messages), len(a.steps))
		}
	})

	t.Run("no snapshotted steps is a no-op", func(t *testing.T) {
		a := newUndoAgent([]StepRecord{{MessageCount: 1}, {MessageCount: 3}}, 5, nil)
		removed, err := a.ReconcileUndo(context.Background(), 1)
		if err != nil || removed != 0 || len(a.Messages) != 5 {
			t.Errorf("got removed=%d err=%v len=%d, want 0/nil/5", removed, err, len(a.Messages))
		}
	})

	t.Run("refuses to undo across a compaction", func(t *testing.T) {
		a := newUndoAgent(mkSteps(), 9, nil)
		a.compactionFloor = 2 // target snapshot (idx1) is below the floor
		if _, err := a.ReconcileUndo(context.Background(), 2); err == nil {
			t.Error("expected refusal to undo across a compaction")
		}
		if len(a.Messages) != 9 {
			t.Error("transcript must be untouched when undo is refused")
		}
	})

	t.Run("refuses when in-memory and persisted counts diverge", func(t *testing.T) {
		// Simulates a resumed/branched/repaired session: disk has fewer rows
		// than the in-memory transcript, so an in-memory boundary would delete
		// the wrong persisted rows. ReconcileUndo must refuse and mutate nothing.
		fp := &fakePersister{diskCount: 5} // a.Messages has 9 → misaligned
		a := newUndoAgent(mkSteps(), 9, fp)
		if _, err := a.ReconcileUndo(context.Background(), 1); err == nil {
			t.Error("expected refusal when transcript is not disk-aligned")
		}
		if len(a.Messages) != 9 || len(a.steps) != 4 || fp.truncCalls != 0 {
			t.Errorf("nothing must be mutated on misalignment: msgs=%d steps=%d truncCalls=%d",
				len(a.Messages), len(a.steps), fp.truncCalls)
		}
	})

	t.Run("rejects non-positive n", func(t *testing.T) {
		a := newUndoAgent(mkSteps(), 9, nil)
		if _, err := a.ReconcileUndo(context.Background(), 0); err == nil {
			t.Error("expected error for n=0")
		}
	})
}
