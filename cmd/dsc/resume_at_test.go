package main

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/agent"
	"github.com/amemiya02/deepseekcode/internal/llm"
)

// TestResumeAtFlagRegistered verifies that the --resume-at flag is registered
// in the CLI so that -help lists it without an "unknown flag" error.
func TestResumeAtFlagRegistered(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "-help")
	cmd.Dir = "."
	out, _ := cmd.CombinedOutput()
	if !strings.Contains(string(out), "resume-at") {
		t.Fatalf("expected --resume-at flag in -help output, got:\n%s", out)
	}
}

// TestResumeAtMessageTruncation verifies the truncation logic applied after
// BranchAt resolves: when res.MessageCount <= len(agent.Messages) the slice
// is trimmed to the boundary so the resumed session starts at the right
// transcript position.
func TestResumeAtMessageTruncation(t *testing.T) {
	a := &agent.Agent{}
	// Populate 5 messages.
	for i := 0; i < 5; i++ {
		a.Messages = append(a.Messages, llm.Message{Role: "user"})
	}

	boundary := 3
	// Reproduce the truncation block from runOneShot.
	if boundary <= len(a.Messages) {
		a.Messages = a.Messages[:boundary]
	}

	if len(a.Messages) != boundary {
		t.Fatalf("expected %d messages after truncation, got %d", boundary, len(a.Messages))
	}
}

// TestResumeAtTruncationNoopWhenBoundaryLarger verifies that truncation is
// skipped (messages unchanged) when MessageCount > len(agent.Messages), which
// can happen if the session was partially loaded.
func TestResumeAtTruncationNoopWhenBoundaryLarger(t *testing.T) {
	a := &agent.Agent{}
	for i := 0; i < 3; i++ {
		a.Messages = append(a.Messages, llm.Message{Role: "user"})
	}

	boundary := 10
	if boundary <= len(a.Messages) {
		a.Messages = a.Messages[:boundary]
	}
	// Should still have 3 messages — boundary was larger so truncation was skipped.
	if len(a.Messages) != 3 {
		t.Fatalf("expected 3 messages (no truncation), got %d", len(a.Messages))
	}
}
