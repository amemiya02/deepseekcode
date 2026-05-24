package hooks

import (
	"context"
	"testing"
	"time"
)

func TestShellCommandUnix(t *testing.T) {
	name, args := shellCommand("echo hello")
	if name != "/bin/sh" {
		t.Fatalf("expected /bin/sh, got %q", name)
	}
	if len(args) != 2 || args[0] != "-c" || args[1] != "echo hello" {
		t.Fatalf("expected [-c echo hello], got %v", args)
	}
}

func TestRunSubprocessHookAllow(t *testing.T) {
	cfg := HookConfig{
		Event:   EventPreToolUse,
		Type:    TypeSubprocess,
		Command: `printf '{"decision":"allow","reason":"ok"}'`,
		Timeout: 5 * time.Second,
	}
	in := HookInput{ToolName: "bash", CWD: "/tmp"}

	out, err := runSubprocessHook(context.Background(), cfg, in)
	if err != nil {
		t.Fatal(err)
	}
	if out.Decision != "allow" {
		t.Fatalf("expected allow, got %q", out.Decision)
	}
}

func TestRunSubprocessHookDeny(t *testing.T) {
	cfg := HookConfig{
		Event:   EventPreToolUse,
		Type:    TypeSubprocess,
		Command: `printf '{"decision":"deny","reason":"nope"}'`,
		Timeout: 5 * time.Second,
	}
	in := HookInput{ToolName: "rm", CWD: "/tmp"}

	out, err := runSubprocessHook(context.Background(), cfg, in)
	if err != nil {
		t.Fatal(err)
	}
	if out.Decision != "deny" {
		t.Fatalf("expected deny, got %q", out.Decision)
	}
}

func TestRunSubprocessHookTimeout(t *testing.T) {
	cfg := HookConfig{
		Event:   EventPreToolUse,
		Type:    TypeSubprocess,
		Command: `sleep 5`,
		Timeout: 100 * time.Millisecond,
	}
	in := HookInput{ToolName: "bash", CWD: "/tmp"}

	out, err := runSubprocessHook(context.Background(), cfg, in)
	if err != nil {
		t.Fatal(err)
	}
	if out.Decision != "continue" {
		t.Fatalf("expected continue on timeout, got %q", out.Decision)
	}
	if out.Reason != "hook timed out" {
		t.Fatalf("expected 'hook timed out', got %q", out.Reason)
	}
}

func TestRunSubprocessHookBadJSON(t *testing.T) {
	cfg := HookConfig{
		Event:   EventPreToolUse,
		Type:    TypeSubprocess,
		Command: `echo not-json`,
		Timeout: 5 * time.Second,
	}
	in := HookInput{ToolName: "bash", CWD: "/tmp"}

	out, err := runSubprocessHook(context.Background(), cfg, in)
	if err != nil {
		t.Fatal(err)
	}
	if out.Decision != "continue" {
		t.Fatalf("expected continue on bad output, got %q", out.Decision)
	}
}

func TestRunSubprocessHookNonZeroExit(t *testing.T) {
	cfg := HookConfig{
		Event:   EventPreToolUse,
		Type:    TypeSubprocess,
		Command: `echo '{"decision":"allow"}'; exit 1`,
		Timeout: 5 * time.Second,
	}
	in := HookInput{ToolName: "bash", CWD: "/tmp"}

	out, err := runSubprocessHook(context.Background(), cfg, in)
	if err != nil {
		t.Fatal(err)
	}
	// Non-zero exit but valid JSON stdout — should still parse the output.
	if out.Decision != "allow" {
		t.Fatalf("expected allow despite non-zero exit, got %q", out.Decision)
	}
}

func TestRunSubprocessHookDefaultTimeout(t *testing.T) {
	cfg := HookConfig{
		Event:   EventPreToolUse,
		Type:    TypeSubprocess,
		Command: `printf '{"decision":"allow"}'`,
		// Timeout is zero — should default to 30s.
	}
	in := HookInput{ToolName: "bash", CWD: "/tmp"}

	out, err := runSubprocessHook(context.Background(), cfg, in)
	if err != nil {
		t.Fatal(err)
	}
	if out.Decision != "allow" {
		t.Fatalf("expected allow, got %q", out.Decision)
	}
}
