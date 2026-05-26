package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/worktree"
)

// WorktreeManager is the narrow interface for worktree management.
// Implemented by *worktree.Manager.
type WorktreeManager interface {
	Create(ctx context.Context, branch, baseRev string) (worktree.Worktree, error)
	Remove(ctx context.Context, branch string, force bool) error
	List(ctx context.Context) ([]worktree.Worktree, error)
	Prune(ctx context.Context) error
	Status(ctx context.Context, branch string) (worktree.Worktree, error)
}

// WorktreeTool exposes git worktree management as a tool.
type WorktreeTool struct {
	m WorktreeManager
}

// NewWorktreeTool creates a WorktreeTool backed by the given manager.
func NewWorktreeTool(m WorktreeManager) *WorktreeTool {
	return &WorktreeTool{m: m}
}

func (*WorktreeTool) Name() string { return "worktree" }

func (*WorktreeTool) Description() string {
	return "Manage git worktrees for isolated sub-agent workspaces. " +
		"Actions: create (new branch+worktree), list (show all), " +
		"remove (delete worktree), prune (clean stale entries), " +
		"status (check a worktree's state)."
}

func (*WorktreeTool) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "Action: create, list, remove, prune, or status",
				"enum":        []string{"create", "list", "remove", "prune", "status"},
			},
			"branch": map[string]any{
				"type":        "string",
				"description": "Branch name (required for create/remove/status)",
			},
			"base_rev": map[string]any{
				"type":        "string",
				"description": "Base revision for create (default: HEAD)",
			},
			"force": map[string]any{
				"type":        "boolean",
				"description": "Force removal even if worktree is dirty (default: false)",
			},
		},
		"required": []string{"action"},
	})
}

func (*WorktreeTool) IsReadOnly() bool { return false }

func (t *WorktreeTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		Action  string `json:"action"`
		Branch  string `json:"branch"`
		BaseRev string `json:"base_rev"`
		Force   bool   `json:"force"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("worktree: invalid args: %v", err), nil
	}
	if p.Action == "" {
		return Errf("action is required"), nil
	}
	if t.m == nil {
		return Errf("worktree: no manager configured"), nil
	}

	switch p.Action {
	case "create":
		if p.Branch == "" {
			return Errf("branch is required for create"), nil
		}
		if p.BaseRev == "" {
			p.BaseRev = "HEAD"
		}
		wt, err := t.m.Create(ctx, p.Branch, p.BaseRev)
		if err != nil {
			if strings.Contains(err.Error(), "invalid branch") {
				return Errf("invalid branch: %v", err), nil
			}
			return Errf("worktree create failed: %v", err), nil
		}
		return Result{
			Content: fmt.Sprintf("created worktree %s at %s (head: %s)", wt.Branch, wt.Path, wt.Head),
		}, nil

	case "list":
		list, err := t.m.List(ctx)
		if err != nil {
			return Errf("worktree list failed: %v", err), nil
		}
		var sb strings.Builder
		sb.WriteString("branch  path  head  locked\n")
		for _, wt := range list {
			locked := "false"
			if wt.Locked {
				locked = "true"
			}
			head := wt.Head
			if len(head) > 7 {
				head = head[:7]
			}
			sb.WriteString(fmt.Sprintf("%s  %s  %s  %s\n",
				wt.Branch, wt.Path, head, locked))
		}
		return Result{Content: sb.String()}, nil

	case "remove":
		if p.Branch == "" {
			return Errf("branch is required for remove"), nil
		}
		if err := t.m.Remove(ctx, p.Branch, p.Force); err != nil {
			return Errf("worktree remove failed: %v", err), nil
		}
		return Result{
			Content: fmt.Sprintf("removed worktree %s", p.Branch),
		}, nil

	case "prune":
		if err := t.m.Prune(ctx); err != nil {
			return Errf("worktree prune failed: %v", err), nil
		}
		return Result{
			Content: "pruned stale worktree entries",
		}, nil

	case "status":
		if p.Branch == "" {
			return Errf("branch is required for status"), nil
		}
		wt, err := t.m.Status(ctx, p.Branch)
		if err != nil {
			return Errf("worktree status failed: %v", err), nil
		}
		locked := "false"
		if wt.Locked {
			locked = "true"
		}
		dirty := "false"
		if wt.Dirty {
			dirty = "true"
		}
		return Result{
			Content: fmt.Sprintf("%s: head=%s locked=%s dirty=%s", wt.Branch, wt.Head, locked, dirty),
		}, nil

	default:
		return Errf("unknown action: %s", p.Action), nil
	}
}
