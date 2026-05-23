package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// GitDiff returns the diff between two refs (or working tree against HEAD).
// Output is the raw unified diff. It's "structured" in that we keep it
// scoped (single path, single ref range) so the model doesn't drown in
// whole-repo diffs.
type GitDiff struct{}

func (GitDiff) Name() string { return "git_diff" }

func (GitDiff) Description() string {
	return "Show a unified diff between two git refs, or between the working tree and HEAD. " +
		"Optional ref_a and ref_b set the range (default: ref_a=HEAD, ref_b=working tree). " +
		"Optional path restricts to a single file or directory. " +
		"Returns the raw unified diff. Cleaner than running `git diff` in bash because output " +
		"is bounded to relevant context."
}

func (GitDiff) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ref_a": map[string]any{
				"type":        "string",
				"description": "Older ref (default HEAD).",
			},
			"ref_b": map[string]any{
				"type":        "string",
				"description": "Newer ref (default: working tree).",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Optional path restriction.",
			},
		},
	})
}

func (GitDiff) IsReadOnly() bool { return true }

func (GitDiff) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		RefA string `json:"ref_a"`
		RefB string `json:"ref_b"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("invalid args: %v", err), nil
	}
	cmdArgs := []string{"diff", "--no-color"}
	if p.RefA != "" && p.RefB != "" {
		cmdArgs = append(cmdArgs, p.RefA, p.RefB)
	} else if p.RefA != "" {
		cmdArgs = append(cmdArgs, p.RefA)
	}
	if p.Path != "" {
		cmdArgs = append(cmdArgs, "--", p.Path)
	}

	out, err := runGit(ctx, cmdArgs...)
	if err != nil {
		return Errf("git diff: %v", err), nil
	}
	if strings.TrimSpace(out) == "" {
		return Result{Content: "(no diff)"}, nil
	}
	return Result{Content: out}, nil
}

// runGit shells out to git and returns combined stdout+stderr. Used by
// all four structured git tools. Errors include the git output so the
// model sees it.
func runGit(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return buf.String(), fmt.Errorf("exit %d: %s", ee.ExitCode(), strings.TrimSpace(buf.String()))
		}
		return buf.String(), err
	}
	return buf.String(), nil
}
