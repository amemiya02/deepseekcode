package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// fakeDuetClient scripts ValidatePro behavior per call index.
type fakeDuetClient struct {
	calls atomic.Int32
	fn    func(ctx context.Context, call int) (bool, string, error)
}

func (f *fakeDuetClient) ValidatePro(ctx context.Context, prompt string) (bool, string, error) {
	n := int(f.calls.Add(1))
	return f.fn(ctx, n)
}

// destructiveInput is a HookInput that isDestructiveCall classifies as
// destructive (rm -rf via bash), so the hook reaches ValidatePro.
func destructiveInput() HookInput {
	return HookInput{
		Event:     EventPreToolUse,
		ToolName:  "bash",
		ToolInput: json.RawMessage(`{"command":"rm -rf /tmp/x"}`),
	}
}

func flashModel() string      { return "deepseek-v4-flash" }
func emptyTranscript() []byte { return nil }

// TestDuetRetryOnFailureRetriesExactlyOnce pins duet.retry_on_failure:
// a transient ValidatePro error is retried once; the second answer wins.
func TestDuetRetryOnFailureRetriesExactlyOnce(t *testing.T) {
	client := &fakeDuetClient{fn: func(_ context.Context, call int) (bool, string, error) {
		if call == 1 {
			return false, "", errors.New("transient")
		}
		return true, "looks intentional", nil
	}}
	hook := NewDuetHookWithOptions(client, nil, t.TempDir(), nil, flashModel, emptyTranscript,
		DuetOptions{RetryOnFailure: true})

	out, err := hook(context.Background(), destructiveInput())
	if err != nil {
		t.Fatalf("hook: %v", err)
	}
	if out.Decision != "allow" {
		t.Fatalf("Decision = %q, want allow (retry should have succeeded)", out.Decision)
	}
	if n := client.calls.Load(); n != 2 {
		t.Fatalf("ValidatePro called %d times, want 2", n)
	}
}

// TestDuetNoRetryFailsOpenAfterFirstError pins the zero-value (and
// retry_on_failure=false) behavior: one failure → fail-open, no retry.
func TestDuetNoRetryFailsOpenAfterFirstError(t *testing.T) {
	client := &fakeDuetClient{fn: func(_ context.Context, _ int) (bool, string, error) {
		return false, "", errors.New("transient")
	}}
	hook := NewDuetHookWithOptions(client, nil, t.TempDir(), nil, flashModel, emptyTranscript,
		DuetOptions{RetryOnFailure: false})

	out, err := hook(context.Background(), destructiveInput())
	if err != nil {
		t.Fatalf("hook: %v", err)
	}
	if out.Decision != "continue" {
		t.Fatalf("Decision = %q, want continue (fail-open)", out.Decision)
	}
	if n := client.calls.Load(); n != 1 {
		t.Fatalf("ValidatePro called %d times, want 1", n)
	}
}

// TestDuetValidatorTimeoutBoundsTheCall pins duet.validator_timeout_ms:
// a ValidatePro that honors ctx cancellation is cut off by the per-call
// timeout and the hook fails open promptly instead of hanging.
func TestDuetValidatorTimeoutBoundsTheCall(t *testing.T) {
	client := &fakeDuetClient{fn: func(ctx context.Context, _ int) (bool, string, error) {
		<-ctx.Done()
		return false, "", ctx.Err()
	}}
	hook := NewDuetHookWithOptions(client, nil, t.TempDir(), nil, flashModel, emptyTranscript,
		DuetOptions{ValidatorTimeout: 50 * time.Millisecond})

	start := time.Now()
	out, err := hook(context.Background(), destructiveInput())
	if err != nil {
		t.Fatalf("hook: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("hook took %v, want prompt fail-open under the 50ms validator timeout", elapsed)
	}
	if out.Decision != "continue" {
		t.Fatalf("Decision = %q, want continue (fail-open on timeout)", out.Decision)
	}
}
