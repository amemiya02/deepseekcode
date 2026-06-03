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
	StepIdx      int
	MessageCount int
}

// resolveBranchBoundary resolves a name-or-integer to a message-count boundary.
// nameOrTurn may be a checkpoint name (looked up from CheckpointIndex) or a
// decimal step index.
func (a *Agent) resolveBranchBoundary(nameOrTurn string) (int, error) {
	// Try integer first.
	if idx, err := strconv.Atoi(nameOrTurn); err == nil {
		if idx < 0 || idx >= len(a.steps) {
			return 0, fmt.Errorf("step %d out of range [0, %d)", idx, len(a.steps))
		}
		return a.steps[idx].MessageCount, nil
	}
	// Fall back to named checkpoint.
	si, ok := a.checkpoints.Lookup(nameOrTurn)
	if !ok {
		return 0, fmt.Errorf("checkpoint %q not found; known: %v", nameOrTurn, a.checkpoints.Names())
	}
	if si < 0 || si >= len(a.steps) {
		return 0, fmt.Errorf("checkpoint %q points to step %d, which is out of range", nameOrTurn, si)
	}
	return a.steps[si].MessageCount, nil
}

// BranchAt forks the current session at the step identified by nameOrTurn
// (a checkpoint name or a decimal step index). It:
//  1. Resolves nameOrTurn to a message-count boundary via resolveBranchBoundary.
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
	boundary, err := a.resolveBranchBoundary(nameOrTurn)
	if err != nil {
		return BranchResult{}, err
	}

	// Derive step index for the result.
	stepIdx := -1
	for i, s := range a.steps {
		if s.MessageCount == boundary {
			stepIdx = i
			break
		}
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

// sanitizeBranchName replaces characters not allowed in git branch names with '-'.
func sanitizeBranchName(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '/':
			out[i] = c
		default:
			out[i] = '-'
		}
	}
	return string(out)
}
