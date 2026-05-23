package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// GitShow reads a file's content at a specific ref, or shows the
// content of a commit. Lets the model reason about historical state
// without checking out a different branch.
type GitShow struct{}

func (GitShow) Name() string { return "git_show" }

func (GitShow) Description() string {
	return "Show a file's content at a specific git ref (e.g. 'HEAD~3:pkg/auth/jwt.go') " +
		"or show a commit's metadata + diff (e.g. ref='abc1234' without path). " +
		"Returns the raw output. Use this to compare current vs past file state " +
		"without leaving the current branch."
}

func (GitShow) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ref": map[string]any{
				"type":        "string",
				"description": "Git ref (commit SHA, branch, tag, HEAD~N).",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Optional file path. When set, shows the file content at that ref.",
			},
		},
		"required": []string{"ref"},
	})
}

func (GitShow) IsReadOnly() bool { return true }

func (GitShow) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		Ref  string `json:"ref"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("invalid args: %v", err), nil
	}
	if p.Ref == "" {
		return Errf("ref is required"), nil
	}
	var target string
	if p.Path != "" {
		target = fmt.Sprintf("%s:%s", p.Ref, p.Path)
	} else {
		target = p.Ref
	}
	out, err := runGit(ctx, "show", "--no-color", target)
	if err != nil {
		return Errf("git show: %v", err), nil
	}
	return Result{Content: out}, nil
}
