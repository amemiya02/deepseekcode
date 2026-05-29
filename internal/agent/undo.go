package agent

import (
	"context"
	"fmt"
)

// MessageTruncator is an optional Persister capability: drop persisted body
// messages with index >= keepCount so disk matches the in-memory transcript
// after an /undo (T3.5). internal/session.Persister implements it; it is
// checked via type assertion so non-persisting agents and test fakes need not
// implement it (mirrors ReceiptAppender).
type MessageTruncator interface {
	TruncateMessages(ctx context.Context, keepCount int) (int, error)
	// PersistedMessageCount returns the count of persisted body messages.
	// ReconcileUndo compares it to len(a.Messages) to confirm the in-memory
	// transcript is index-aligned with disk before truncating disk — alignment
	// breaks after a resume, a branch, or a dangling-tool-call repair insert,
	// where an in-memory boundary would delete the wrong persisted rows.
	PersistedMessageCount(ctx context.Context) (int, error)
}

// ReconcileUndo rolls the transcript back by n completed steps so the model's
// view matches the files a /undo just reverted (today /undo reverts files only;
// a.Messages and disk still claim the reverted turn succeeded). It truncates
// a.Messages to the boundary the first undone step started from
// (StepRecord.MessageCount), trims a.steps in lockstep, and — when the Persister
// supports it — truncates the persisted messages to the same boundary.
//
// It refuses to cross a compaction: compaction renumbers a.Messages, so
// boundaries recorded by steps below compactionFloor are stale and truncating
// to them would corrupt the transcript. The caller (TUI) must ensure the agent
// is not running, since ReconcileUndo mutates a.Messages that runStep reads.
//
// The Static Prefix (system + tools) is untouched, so the cache fingerprint is
// byte-identical across the undo and the 50x discount survives the rewind.
// Returns the number of body messages removed.
func (a *Agent) ReconcileUndo(ctx context.Context, n int) (int, error) {
	if n <= 0 {
		return 0, fmt.Errorf("undo count must be positive, got %d", n)
	}

	// /undo counts SNAPSHOTS (mutating steps), matching the snapshot manager's
	// file revert — snapshots are sparse (read-only steps take none), so the
	// raw step count would diverge from the file undo. Walk a.steps backward
	// to the n-th snapshotted step; that step's boundary is where to roll to.
	target := -1
	seen := 0
	for i := len(a.steps) - 1; i >= 0; i-- {
		if !a.steps[i].Snapshotted {
			continue
		}
		seen++
		if seen == n {
			target = i
			break
		}
	}
	if target < 0 {
		// n exceeds the snapshots taken this session. Manager.Undo clamps to
		// the number of snapshots and reverts them all, so roll to the earliest
		// snapshotted step. If none exist, the file undo would have errored
		// before reaching here; treat as a no-op for safety.
		for i := 0; i < len(a.steps); i++ {
			if a.steps[i].Snapshotted {
				target = i
				break
			}
		}
		if target < 0 {
			return 0, nil
		}
	}
	if target < a.compactionFloor {
		return 0, fmt.Errorf("cannot undo across a compaction (transcript was rewritten at step %d)", a.compactionFloor)
	}

	boundary := a.steps[target].MessageCount
	if boundary > len(a.Messages) {
		// A recorded boundary must never exceed the live transcript; refuse
		// rather than risk a corrupt slice.
		return 0, fmt.Errorf("undo boundary %d exceeds transcript length %d", boundary, len(a.Messages))
	}

	// Before mutating anything, verify the in-memory transcript is index-aligned
	// with disk. The recorded boundary is an in-memory message index; the
	// persisted truncation deletes rows by disk idx. These coincide only in a
	// session whose transcript was built entirely by the live append path —
	// NOT after a resume (a.Messages rebuilt by Replay), a branch (parent+child
	// concatenation), or a dangling-tool-call repair (synthetic in-memory rows
	// with no disk row). Refusing when they diverge keeps undo from deleting the
	// wrong persisted rows; the files were already reverted, so the caller warns
	// that the model's view is unchanged. (Nothing is mutated before this gate.)
	trunc, hasTrunc := a.Persister.(MessageTruncator)
	if hasTrunc {
		diskCount, err := trunc.PersistedMessageCount(ctx)
		if err != nil {
			return 0, fmt.Errorf("undo: cannot verify transcript alignment: %w", err)
		}
		if diskCount != len(a.Messages) {
			return 0, fmt.Errorf("cannot reconcile transcript: in-memory (%d) and persisted (%d) message counts differ — likely a resumed or branched session; the files were reverted but the model's view is unchanged", len(a.Messages), diskCount)
		}
	}

	removed := len(a.Messages) - boundary
	a.Messages = a.Messages[:boundary]
	a.steps = a.steps[:target]

	if hasTrunc {
		if _, err := trunc.TruncateMessages(ctx, boundary); err != nil {
			return removed, fmt.Errorf("transcript reverted in memory but persisting the truncation failed: %w", err)
		}
	}
	return removed, nil
}
