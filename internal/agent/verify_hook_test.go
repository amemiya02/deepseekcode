package agent

import (
	"context"
	"os/exec"
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
	if _, err := exec.LookPath("true"); err != nil {
		t.Skip("unix `true` not available")
	}
	h := &VerifyHook{Cmd: "definitely-not-a-real-binary-xyzzy"}
	_, passed := h.Run(context.Background())
	if passed {
		t.Fatal("expected fail for missing binary")
	}
}
