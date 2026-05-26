package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// JobController is the narrow interface tools use to start background
// jobs. Implemented by *agent.Agent.
type JobController interface {
	StartBashJob(ctx context.Context, command string, usePTY bool, timeoutMs int) (jobID string, err error)
}

// BackgroundBashTool runs a shell command in the background and returns immediately.
type BackgroundBashTool struct {
	c JobController
}

// NewBackgroundBashTool creates a BackgroundBashTool backed by the given controller.
func NewBackgroundBashTool(c JobController) *BackgroundBashTool {
	return &BackgroundBashTool{c: c}
}

func (*BackgroundBashTool) Name() string { return "background_bash" }

func (*BackgroundBashTool) Description() string {
	return "Run a shell command in the background. Returns immediately with a job ID; " +
		"use task_status to poll for completion and output. Use for long-running commands " +
		"that should not block the current turn. Default timeout 600000ms (10 minutes)."
}

func (*BackgroundBashTool) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command":    map[string]any{"type": "string", "description": "The shell command to run"},
			"pty":        map[string]any{"type": "boolean", "description": "Run in a pseudo-terminal (for interactive commands)", "default": false},
			"timeout_ms": map[string]any{"type": "integer", "description": "Timeout in milliseconds", "default": 600000},
		},
		"required": []string{"command"},
	})
}

func (t *BackgroundBashTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		Command   string `json:"command"`
		PTY       bool   `json:"pty"`
		TimeoutMs int    `json:"timeout_ms"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("background_bash: invalid args: %v", err), nil
	}
	if p.Command == "" {
		return Errf("command is required"), nil
	}
	if t.c == nil {
		return Errf("background_bash: no job controller configured"), nil
	}
	if p.TimeoutMs <= 0 {
		p.TimeoutMs = 600000 // 10 minutes default
	}

	jobID, err := t.c.StartBashJob(ctx, p.Command, p.PTY, p.TimeoutMs)
	if err != nil {
		return Errf("failed to start background job: %v", err), nil
	}

	return Result{
		Content: fmt.Sprintf("started job %s (use task_status to poll)", jobID),
	}, nil
}

// IsReadOnly returns false - background_bash can modify the filesystem.
func (*BackgroundBashTool) IsReadOnly() bool { return false }
