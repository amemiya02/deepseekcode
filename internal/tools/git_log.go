package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// GitLog returns recent commits as structured one-liners. Default 20
// commits; pass `n` to override. Optional path scopes to commits that
// touched that file/dir; optional since (e.g. '2 weeks ago') bounds time.
type GitLog struct{}

func (GitLog) Name() string { return "git_log" }

func (GitLog) Description() string {
	return "Show recent commits in compact format: 'SHA8 YYYY-MM-DD Author: subject'. " +
		"Default 20 commits. Optional path scopes to commits touching that file/dir; " +
		"optional since accepts git's relative dates ('2 weeks ago', 'yesterday') or absolute. " +
		"Use this to scan recent activity in an area before making changes."
}

func (GitLog) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Optional path scope.",
			},
			"n": map[string]any{
				"type":        "integer",
				"description": "Number of commits to return (default 20, max 200).",
			},
			"since": map[string]any{
				"type":        "string",
				"description": "Optional time bound (e.g. '2 weeks ago').",
			},
		},
	})
}

func (GitLog) IsReadOnly() bool { return true }

func (GitLog) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		Path  string `json:"path"`
		N     int    `json:"n"`
		Since string `json:"since"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("invalid args: %v", err), nil
	}
	if p.N <= 0 {
		p.N = 20
	}
	if p.N > 200 {
		p.N = 200
	}
	cmdArgs := []string{
		"log",
		"--no-color",
		"--pretty=format:%h %ad %an: %s",
		"--date=short",
		fmt.Sprintf("-n%d", p.N),
	}
	if p.Since != "" {
		cmdArgs = append(cmdArgs, "--since="+p.Since)
	}
	if p.Path != "" {
		cmdArgs = append(cmdArgs, "--", p.Path)
	}
	out, err := runGit(ctx, cmdArgs...)
	if err != nil {
		return Errf("git log: %v", err), nil
	}
	return Result{Content: out}, nil
}
