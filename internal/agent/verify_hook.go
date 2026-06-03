package agent

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// VerifyHook runs a shell command after mutating steps and reports whether
// the working tree is in a good state. It is configured via Agent.Verify and
// called from two points in the agent loop:
//
//  1. After every step that includes at least one mutating tool call
//     (via maybeRunVerifyHook): on failure the feedback is injected as a
//     synthetic user message so the model can fix errors on its next turn.
//  2. At model-stop time when the model emits no tool calls: on success the
//     stop reason is promoted from StopModelDone to StopVerifiedDone; on
//     failure the feedback is injected into a.Messages before returning.
//
// When Cmd is empty, the hook is disabled and always reports pass.
type VerifyHook struct {
	// Cmd is the shell command to run (e.g. "go build ./..." or "go test ./...").
	// Executed via sh -c so shell expansion is supported.
	Cmd string
	// Shell is the interpreter. Defaults to "sh" if empty.
	Shell string
}

// Run executes the verify command and returns (feedback, passed).
//   - passed=true, feedback="" when the command exits 0 or Cmd is empty.
//   - passed=false, feedback=<synthesized message> when the command exits non-0.
func (h *VerifyHook) Run(ctx context.Context) (feedback string, passed bool) {
	if h.Cmd == "" {
		return "", true
	}
	shell := h.Shell
	if shell == "" {
		shell = "sh"
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, shell, "-c", h.Cmd)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		outStr := strings.TrimSpace(stdout.String())
		errStr := strings.TrimSpace(stderr.String())
		var combined string
		switch {
		case outStr != "" && errStr != "":
			combined = outStr + "\n" + errStr
		case outStr != "":
			combined = outStr
		default:
			combined = errStr
		}
		feedback = fmt.Sprintf(
			"Verification failed (command: %q).\n\nOutput:\n%s\n\n"+
				"Please fix the above errors before continuing.",
			h.Cmd, combined,
		)
		return feedback, false
	}
	return "", true
}
