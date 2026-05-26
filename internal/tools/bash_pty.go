package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/amemiya02/deepseekcode/internal/sandbox"
)

// BashPTY runs a shell command inside a pseudo-terminal (PTY).
// Use when a command requires a TTY (interactive prompts disabled, color/curses output).
type BashPTY struct {
	Sandbox        sandbox.Sandbox
	SandboxProfile sandbox.Profile
	CWD            string
}

func (BashPTY) Name() string { return "bash_pty" }

func (BashPTY) Description() string {
	return "Run a shell command inside a pseudo-terminal (PTY). " +
		"Use when a command requires a TTY (interactive prompts disabled, color/curses output). " +
		"Output is captured stdout+stderr from the PTY, with TERM=dumb. " +
		"Default timeout 120000ms. For plain commands prefer bash."
}

func (BashPTY) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The shell command to run in a PTY.",
			},
			"timeout_ms": map[string]any{
				"type":        "integer",
				"description": "Optional timeout in milliseconds. Default 120000 (2 min).",
			},
		},
		"required": []string{"command"},
	})
}

// AffectedPaths returns nil — bash_pty effects are unknown statically.
func (BashPTY) AffectedPaths(args json.RawMessage) []string { return nil }

func (b BashPTY) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
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

	timeout := time.Duration(p.TimeoutMs) * time.Millisecond
	start := time.Now()
	profile := sandboxProfileWithCWD(b.SandboxProfile, b.CWD)
	output, exitCode, err := runPTYWithSandbox(ctx, p.Command, timeout, b.Sandbox, profile)
	dur := time.Since(start)

	// Handle PTY unsupported (Windows)
	if errors.Is(err, errPTYUnsupported) {
		return Errf("bash_pty: not supported on this platform"), nil
	}

	// Handle timeout
	if errors.Is(err, context.DeadlineExceeded) {
		return Errf("command timed out after %s\noutput so far:\n%s", dur.Round(time.Millisecond), output), nil
	}

	// Handle other errors
	if err != nil {
		return Result{}, fmt.Errorf("running pty command: %w", err)
	}

	// Success or non-zero exit
	if exitCode == 0 {
		return Result{
			Content: fmt.Sprintf("(exit 0 in %s)\n%s", dur.Round(time.Millisecond), output),
		}, nil
	}

	blocked, body := classifyBlocked(b.Sandbox, output)
	if blocked {
		return Result{Content: body, IsError: true}, nil
	}
	return Result{
		Content: fmt.Sprintf("exit status %d (in %s)\n%s", exitCode, dur.Round(time.Millisecond), body),
		IsError: true,
	}, nil
}
