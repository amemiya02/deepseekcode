package agent

import (
	"context"
	"fmt"
	"strconv"

	"github.com/amemiya02/deepseekcode/internal/worktree"
)

// BranchResult is returned by BranchAt to the caller (TUI/CLI).
type BranchResult struct {
	WorktreePath string
	Branch       string
	// StepIdx is the zero-based index into a.steps of the resolved boundary.
	StepIdx      int
	MessageCount int
}

// resolveBranchBoundary resolves a name-or-integer to a (stepIdx, messageCount)
// pair that identifies the branch point. nameOrTurn may be a checkpoint name
// (looked up from CheckpointIndex) or a decimal step index.
//
// Returns an error if:
//   - The step index / checkpoint name is invalid.
//   - The resolved step index is below a.compactionFloor (the transcript was
//     renumbered by a compaction, so the boundary stored in that StepRecord is
//     stale and cannot be used safely).
func (a *Agent) resolveBranchBoundary(nameOrTurn string) (stepIdx int, messageCount int, err error) {
	// Resolve to a step index first.
	var si int
	if idx, convErr := strconv.Atoi(nameOrTurn); convErr == nil {
		if len(a.steps) == 0 {
			return 0, 0, fmt.Errorf("no steps recorded yet")
		}
		if idx < 0 || idx >= len(a.steps) {
			return 0, 0, fmt.Errorf("step %d out of range [0, %d)", idx, len(a.steps))
		}
		si = idx
	} else {
		// Fall back to named checkpoint.
		var ok bool
		si, ok = a.checkpoints.Lookup(nameOrTurn)
		if !ok {
			return 0, 0, fmt.Errorf("checkpoint %q not found; known: %v", nameOrTurn, a.checkpoints.Names())
		}
		if si < 0 || si >= len(a.steps) {
			return 0, 0, fmt.Errorf("checkpoint %q points to step %d, which is out of range", nameOrTurn, si)
		}
	}

	// Guard against a stale boundary caused by compaction (mirrors undo.go:74).
	if si < a.compactionFloor {
		return 0, 0, fmt.Errorf("cannot branch at step %d: transcript was rewritten by a compaction at step %d (boundaries before that step are stale)", si, a.compactionFloor)
	}

	return si, a.steps[si].MessageCount, nil
}

// BranchAt forks the current session at the step identified by nameOrTurn
// (a checkpoint name or a decimal step index). It:
//  1. Resolves nameOrTurn to a (stepIdx, messageCount) boundary via resolveBranchBoundary.
//  2. Creates a new git worktree via wt.Create (branch name derived from nameOrTurn).
//  3. Returns BranchResult so the TUI/CLI can open the new worktree directory.
//
// BranchAt does NOT truncate a.Messages: the current session continues
// unaffected. The caller is responsible for launching a new dsc process or TUI
// session rooted at BranchResult.WorktreePath.
//
// wt may be nil, in which case BranchAt resolves the boundary but skips
// worktree creation (useful for --dry-run or non-git repos).
func (a *Agent) BranchAt(ctx context.Context, nameOrTurn string, wt *worktree.Manager) (BranchResult, error) {
	stepIdx, boundary, err := a.resolveBranchBoundary(nameOrTurn)
	if err != nil {
		return BranchResult{}, err
	}

	res := BranchResult{
		StepIdx:      stepIdx,
		MessageCount: boundary,
	}

	if wt == nil {
		return res, nil
	}

	branch := fmt.Sprintf("branch/%s", sanitizeBranchName(nameOrTurn))
	newWT, err := wt.Create(ctx, branch, "HEAD")
	if err != nil {
		return BranchResult{}, fmt.Errorf("branch: worktree create failed: %w", err)
	}
	res.WorktreePath = newWT.Path
	res.Branch = newWT.Branch
	return res, nil
}

// sanitizeBranchName replaces characters not allowed in git branch name
// components with '-'. The '/' character is intentionally excluded: callers
// that want hierarchical refs must supply them explicitly (e.g. the "branch/"
// prefix added by BranchAt). Allowing '/' through here would let a nameOrTurn
// such as "feature/foo" silently produce "branch/feature/foo", an extra ref
// level that can surprise callers.
func sanitizeBranchName(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '-', c == '_':
			out[i] = c
		default:
			out[i] = '-'
		}
	}
	return string(out)
}
