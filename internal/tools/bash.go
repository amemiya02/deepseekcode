package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"time"
)

// Bash runs a single shell command in the user's default shell. The
// permission layer must vet `command` before we ever get here.
//
// We always run via the shell rather than parsing the command ourselves
// because the model is going to write `command1 | grep foo && command2`
// constantly and we don't want to reimplement the shell.
type Bash struct{}

func (Bash) Name() string { return "bash" }

func (Bash) Description() string {
	return "Run a shell command and return its stdout+stderr. " +
		"Runs in the user's default shell (sh on Unix, cmd on Windows) in the current working directory. " +
		"Optional timeout_ms caps execution (default 120000). Output is truncated to ~10000 lines if longer. " +
		"Use this for git, build tools, tests, and ad-hoc inspection. " +
		"For file reads use read_file; for content search use grep; for filename matching use glob."
}

func (Bash) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The shell command to run.",
			},
			"timeout_ms": map[string]any{
				"type":        "integer",
				"description": "Optional timeout in milliseconds. Default 120000 (2 min).",
			},
		},
		"required": []string{"command"},
	})
}

// AffectedPaths returns nil — bash effects are unknown statically.
// The snapshot manager handles bash by snapshotting the whole working
// tree (bounded) when a destructive bash pattern matches.
func (Bash) AffectedPaths(args json.RawMessage) []string { return nil }

func (Bash) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		Command   string `json:"command"`
		TimeoutMs int    `json:"timeout_ms"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("invalid args: %v", err), nil
	}
	if p.Command == "" {
		return Errf("command is required"), nil
	}
	if p.TimeoutMs <= 0 {
		p.TimeoutMs = 120_000
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(p.TimeoutMs)*time.Millisecond)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(timeoutCtx, "cmd", "/C", p.Command)
	} else {
		cmd = exec.CommandContext(timeoutCtx, "sh", "-c", p.Command)
	}

	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined

	start := time.Now()
	err := cmd.Run()
	dur := time.Since(start)

	output := truncateOutput(combined.String(), 10_000)

	if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
		return Errf("command timed out after %s\noutput so far:\n%s", dur, output), nil
	}

	if err != nil {
		// Distinguish "ran but exited non-zero" from "couldn't run."
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return Result{
				Content: fmt.Sprintf("exit status %d (in %s)\n%s",
					ee.ExitCode(), dur.Round(time.Millisecond), output),
				IsError: true,
			}, nil
		}
		return Result{}, fmt.Errorf("running command: %w", err)
	}

	return Result{
		Content: fmt.Sprintf("(exit 0 in %s)\n%s", dur.Round(time.Millisecond), output),
	}, nil
}

// truncateOutput keeps the head + tail of long outputs with a marker.
// Models do better with both ends than with just a head chop.
func truncateOutput(s string, maxLines int) string {
	if s == "" {
		return ""
	}
	lines := splitLines(s)
	if len(lines) <= maxLines {
		return s
	}
	keep := maxLines / 2
	head := lines[:keep]
	tail := lines[len(lines)-keep:]
	marker := fmt.Sprintf("\n... (truncated %d lines) ...\n", len(lines)-keep*2)
	return joinLines(head) + marker + joinLines(tail)
}

func splitLines(s string) []string {
	out := make([]string, 0, 64)
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func joinLines(ls []string) string {
	var b bytes.Buffer
	for i, l := range ls {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(l)
	}
	return b.String()
}
