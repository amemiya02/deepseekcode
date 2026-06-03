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

// TestResumeAtMessageTruncation verifies that applyResumeAtTruncation (the
// production helper called by runOneShot) trims the transcript to the
// boundary when messageCount is within bounds.
func TestResumeAtMessageTruncation(t *testing.T) {
	a := &agent.Agent{}
	for i := 0; i < 5; i++ {
		a.Messages = append(a.Messages, llm.Message{Role: "user"})
	}

	boundary := 3
	applyResumeAtTruncation(a, boundary)

	if len(a.Messages) != boundary {
		t.Fatalf("expected %d messages after truncation, got %d", boundary, len(a.Messages))
	}
}

// TestResumeAtTruncationNoopWhenBoundaryLarger verifies that
// applyResumeAtTruncation leaves the transcript unchanged when messageCount
// exceeds the current length (e.g. partially-loaded session).
func TestResumeAtTruncationNoopWhenBoundaryLarger(t *testing.T) {
	a := &agent.Agent{}
	for i := 0; i < 3; i++ {
		a.Messages = append(a.Messages, llm.Message{Role: "user"})
	}

	applyResumeAtTruncation(a, 10)

	// Should still have 3 messages — boundary was larger so truncation was skipped.
	if len(a.Messages) != 3 {
		t.Fatalf("expected 3 messages (no truncation), got %d", len(a.Messages))
	}
}
