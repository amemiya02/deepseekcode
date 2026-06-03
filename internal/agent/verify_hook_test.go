package agent

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func TestVerifyHookPass(t *testing.T) {
	h := &VerifyHook{Cmd: "true"} // unix `true` always exits 0
	feedback, passed := h.Run(context.Background())
	if !passed {
		t.Fatalf("expected pass; feedback: %s", feedback)
	}
	if feedback != "" {
		t.Fatalf("expected empty feedback on pass, got %q", feedback)
	}
}

func TestVerifyHookFail(t *testing.T) {
	h := &VerifyHook{Cmd: "false"} // unix `false` always exits 1
	feedback, passed := h.Run(context.Background())
	if passed {
		t.Fatal("expected fail")
	}
	if feedback == "" {
		t.Fatal("expected non-empty feedback on fail")
	}
	if !strings.Contains(feedback, h.Cmd) {
		t.Fatalf("expected feedback to contain command %q; got: %s", h.Cmd, feedback)
	}
}

func TestVerifyHookDisabled(t *testing.T) {
	h := &VerifyHook{} // Cmd == "" → disabled
	feedback, passed := h.Run(context.Background())
	if !passed {
		t.Fatalf("disabled hook should always pass; feedback: %s", feedback)
	}
	if feedback != "" {
		t.Fatalf("disabled hook should return empty feedback, got %q", feedback)
	}
}

func TestVerifyHookCommandNotFound(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	h := &VerifyHook{Cmd: "definitely-not-a-real-binary-xyzzy"}
	_, passed := h.Run(context.Background())
	if passed {
		t.Fatal("expected fail for missing binary")
	}
}

func TestVerifyHookFeedbackMessage(t *testing.T) {
	h := &VerifyHook{Cmd: "false"}
	fb, passed := h.Run(context.Background())
	if passed {
		t.Fatal("expected fail")
	}
	// Feedback must mention the command.
	if !strings.Contains(fb, "false") {
		t.Errorf("feedback %q should mention command", fb)
	}
	if !strings.Contains(fb, "Verification failed") {
		t.Errorf("feedback %q should say 'Verification failed'", fb)
	}
}
