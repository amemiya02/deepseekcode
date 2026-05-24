package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"time"
)

// shellCommand returns the shell invocation for cmd.
func shellCommand(cmd string) (name string, args []string) {
	if runtime.GOOS == "windows" {
		return "cmd.exe", []string{"/C", cmd}
	}
	return "/bin/sh", []string{"-c", cmd}
}

// runSubprocessHook spawns a shell -c command, writes HookInput as JSON
// to stdin, reads HookOutput as JSON from stdout. On any failure
// (timeout, non-zero exit, bad output) it returns a "continue" decision
// with the reason so hooks never block the agent loop.
func runSubprocessHook(ctx context.Context, cfg HookConfig, in HookInput) (HookOutput, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	hookCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	inputJSON, err := json.Marshal(in)
	if err != nil {
		return HookOutput{Decision: "continue", Reason: "failed to marshal hook input"}, nil
	}

	name, args := shellCommand(cfg.Command)
	cmd := exec.CommandContext(hookCtx, name, args...)
	cmd.Stdin = bytes.NewReader(inputJSON)

	// Read only stdout; stderr is not parsed as HookOutput.
	// Buffer capped to 64KB to avoid memory exhaustion from a runaway hook.
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	err = cmd.Run()

	// Read stdout regardless of error — a hook may write valid JSON before
	// exiting non-zero.
	if stdout.Len() > 0 {
		// Cap at 64KB.
		raw := stdout.Bytes()
		if len(raw) > 64*1024 {
			raw = raw[:64*1024]
		}

		var out HookOutput
		if jsonErr := json.Unmarshal(raw, &out); jsonErr == nil {
			if out.Decision != "" {
				return out, nil
			}
		}
	}

	if err != nil {
		if hookCtx.Err() == context.DeadlineExceeded {
			return HookOutput{Decision: "continue", Reason: "hook timed out"}, nil
		}
		return HookOutput{Decision: "continue", Reason: fmt.Sprintf("hook error: %v", err)}, nil
	}

	// Process exited 0 but stdout was empty or not valid JSON.
	return HookOutput{Decision: "continue", Reason: "hook produced no valid output"}, nil
}
