package hooks

import (
	"context"
	"testing"
	"time"
)

func TestRunnerNoHooks(t *testing.T) {
	r := NewRunner()
	out, err := r.Run(context.Background(), EventPreToolUse, HookInput{ToolName: "bash", CWD: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Decision != "allow" {
		t.Fatalf("expected allow, got %q", out.Decision)
	}
}

func TestRunnerBuiltinAllow(t *testing.T) {
	r := NewRunner()
	r.Register("echo", func(ctx context.Context, in HookInput) (HookOutput, error) {
		return HookOutput{Decision: "allow", Reason: "looks safe"}, nil
	})
	r.Configure([]HookConfig{
		{Event: EventPreToolUse, Type: TypeBuiltin, Name: "echo"},
	})

	out, err := r.Run(context.Background(), EventPreToolUse, HookInput{ToolName: "read_file"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Decision != "allow" {
		t.Fatalf("expected allow, got %q", out.Decision)
	}
}

func TestRunnerBuiltinDeny(t *testing.T) {
	r := NewRunner()
	r.Register("blocker", func(ctx context.Context, in HookInput) (HookOutput, error) {
		return HookOutput{Decision: "deny", Reason: "forbidden"}, nil
	})
	r.Configure([]HookConfig{
		{Event: EventPreToolUse, Type: TypeBuiltin, Name: "blocker"},
	})

	out, err := r.Run(context.Background(), EventPreToolUse, HookInput{ToolName: "rm"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Decision != "deny" {
		t.Fatalf("expected deny, got %q", out.Decision)
	}
}

func TestRunnerBuiltinAsk(t *testing.T) {
	r := NewRunner()
	r.Register("checker", func(ctx context.Context, in HookInput) (HookOutput, error) {
		return HookOutput{Decision: "ask", Reason: "please confirm"}, nil
	})
	r.Configure([]HookConfig{
		{Event: EventPreToolUse, Type: TypeBuiltin, Name: "checker"},
	})

	out, err := r.Run(context.Background(), EventPreToolUse, HookInput{ToolName: "write_file"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Decision != "ask" {
		t.Fatalf("expected ask, got %q", out.Decision)
	}
}

func TestRunnerMultiHookDenyShortCircuits(t *testing.T) {
	r := NewRunner()
	r.Register("allow-all", func(ctx context.Context, in HookInput) (HookOutput, error) {
		return HookOutput{Decision: "allow"}, nil
	})
	r.Register("deny-rm", func(ctx context.Context, in HookInput) (HookOutput, error) {
		if in.ToolName == "rm" {
			return HookOutput{Decision: "deny", Reason: "no rm"}, nil
		}
		return HookOutput{Decision: "allow"}, nil
	})
	r.Configure([]HookConfig{
		{Event: EventPreToolUse, Type: TypeBuiltin, Name: "allow-all"},
		{Event: EventPreToolUse, Type: TypeBuiltin, Name: "deny-rm"},
	})

	out, err := r.Run(context.Background(), EventPreToolUse, HookInput{ToolName: "rm"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Decision != "deny" {
		t.Fatalf("expected deny (short-circuit), got %q", out.Decision)
	}
}

func TestRunnerMultiHookAskBeforeAllow(t *testing.T) {
	r := NewRunner()
	r.Register("allower", func(ctx context.Context, in HookInput) (HookOutput, error) {
		return HookOutput{Decision: "allow"}, nil
	})
	r.Register("asker", func(ctx context.Context, in HookInput) (HookOutput, error) {
		return HookOutput{Decision: "ask", Reason: "are you sure?"}, nil
	})
	r.Configure([]HookConfig{
		{Event: EventPreToolUse, Type: TypeBuiltin, Name: "allower"},
		{Event: EventPreToolUse, Type: TypeBuiltin, Name: "asker"},
	})

	out, err := r.Run(context.Background(), EventPreToolUse, HookInput{ToolName: "bash"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Decision != "ask" {
		t.Fatalf("expected ask to win over allow, got %q", out.Decision)
	}
}

func TestRunnerBuiltinNotRegistered(t *testing.T) {
	r := NewRunner()
	// Configure a builtin that was never Register-ed.
	r.Configure([]HookConfig{
		{Event: EventPreToolUse, Type: TypeBuiltin, Name: "does-not-exist"},
	})

	out, err := r.Run(context.Background(), EventPreToolUse, HookInput{ToolName: "bash"})
	if err != nil {
		t.Fatal(err)
	}
	// Unregistered builtins are treated as "continue" (fail-open).
	if out.Decision != "allow" {
		t.Fatalf("expected allow after unregistered builtin skipped, got %q", out.Decision)
	}
}

func TestRunnerEventNotConfigured(t *testing.T) {
	r := NewRunner()
	r.Register("echo", func(ctx context.Context, in HookInput) (HookOutput, error) {
		return HookOutput{Decision: "deny"}, nil
	})
	r.Configure([]HookConfig{
		{Event: EventPreToolUse, Type: TypeBuiltin, Name: "echo"},
	})

	// SessionStart has no hook configured, should return allow.
	out, err := r.Run(context.Background(), EventSessionStart, HookInput{CWD: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Decision != "allow" {
		t.Fatalf("expected allow for unconfigured event, got %q", out.Decision)
	}
}

// TestRunnerFailOpenSubprocess verifies that Runner.Run returns
// allow when all subprocess hooks fail (crash, timeout, bad output)
// because fail-open converts errors to "continue".
func TestRunnerFailOpenSubprocess(t *testing.T) {
	r := NewRunner()

	// Crash: process exits non-zero with no stdout.
	r.Configure([]HookConfig{
		{Event: EventPreToolUse, Type: TypeSubprocess, Command: `exit 1`},
	})
	out, err := r.Run(context.Background(), EventPreToolUse, HookInput{ToolName: "bash", CWD: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Decision != "allow" {
		t.Fatalf("crash: expected allow, got %q", out.Decision)
	}

	// Timeout: process sleeps longer than hook timeout.
	r.Configure([]HookConfig{
		{Event: EventPreToolUse, Type: TypeSubprocess, Command: `sleep 5`, Timeout: 50 * time.Millisecond},
	})
	out, err = r.Run(context.Background(), EventPreToolUse, HookInput{ToolName: "bash", CWD: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Decision != "allow" {
		t.Fatalf("timeout: expected allow, got %q", out.Decision)
	}

	// Bad output: process produces invalid JSON.
	r.Configure([]HookConfig{
		{Event: EventPreToolUse, Type: TypeSubprocess, Command: `echo "not json"`},
	})
	out, err = r.Run(context.Background(), EventPreToolUse, HookInput{ToolName: "bash", CWD: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Decision != "allow" {
		t.Fatalf("bad output: expected allow, got %q", out.Decision)
	}
}
