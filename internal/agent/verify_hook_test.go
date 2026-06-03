package agent

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/llmtest"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
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

// newVerifyAgent builds an agent wired to the given mock server with a
// Verify hook set to cmd, using auto-approve permissions and a short step deadline.
func newVerifyAgent(t *testing.T, srv *llmtest.Server, cmd string) *Agent {
	t.Helper()
	reg := tools.New()
	pol := permissions.New(permissions.ModeYolo, t.TempDir(), nil, nil, nil)
	a := New(srv.Client(), reg, pol, "deepseek-v4-flash")
	a.System = "test system"
	a.StepTimeout = 10 * time.Second
	a.Verify = &VerifyHook{Cmd: cmd}
	return a
}

// TestAgentVerifyHookFeedbackInjectedOnMutatingStepFailure is an agent-level
// integration test: when Agent.Verify is set and the verify command fails after
// a mutating tool call, the feedback must be appended to a.Messages.
func TestAgentVerifyHookFeedbackInjectedOnMutatingStepFailure(t *testing.T) {
	// Two turns: first emits a tool call (mutating), second is a plain stop.
	// The verify hook always fails, so after the tool step feedback is injected.
	srv := llmtest.NewServer(
		llmtest.Turn{
			ToolCalls: []llmtest.ToolCall{{ID: "call_1", Name: "echo", Args: `{"text":"hi"}`}},
			Finish:    "tool_calls",
		},
		llmtest.Turn{Text: "done"},
	)
	defer srv.Close()

	var calls int32
	a := newVerifyAgent(t, srv, "false") // "false" always fails
	a.Tools.Register(loopEchoTool{calls: &calls})

	_, err := a.Run(context.Background(), "echo hi")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// At least one user-role message after the initial prompt must contain
	// verify feedback (i.e. the feedback was injected).
	found := false
	for _, m := range a.Messages {
		if m.Role != "user" {
			continue
		}
		for _, b := range m.Blocks {
			if tb, ok := b.(llm.TextBlock); ok && strings.Contains(tb.Text, "Verification failed") {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected a 'Verification failed' feedback message injected into a.Messages after a mutating step, but none found; messages=%+v", a.Messages)
	}
}

// TestAgentVerifyHookPromotesStopReasonOnSuccess is an agent-level integration
// test: when Agent.Verify is set and the verify command passes at model-stop
// time, the stop reason must be StopVerifiedDone (not StopModelDone).
func TestAgentVerifyHookPromotesStopReasonOnSuccess(t *testing.T) {
	// Single turn: model stops immediately with no tool calls.
	srv := llmtest.NewServer(llmtest.Turn{Text: "done"})
	defer srv.Close()

	a := newVerifyAgent(t, srv, "true") // "true" always passes

	reason, err := a.Run(context.Background(), "x")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if reason != StopVerifiedDone {
		t.Errorf("reason = %v, want StopVerifiedDone when verify hook passes at model-stop time", reason)
	}
}

// TestAgentVerifyHookInjectsFeedbackAtStopTimeOnFailure is an agent-level
// integration test: when Agent.Verify is set and the verify command fails at
// model-stop time, the feedback must be present in a.Messages so the caller
// (or a re-entered loop) has context about the failure.
func TestAgentVerifyHookInjectsFeedbackAtStopTimeOnFailure(t *testing.T) {
	// Single turn: model stops immediately with no tool calls, but verify fails.
	srv := llmtest.NewServer(llmtest.Turn{Text: "done"})
	defer srv.Close()

	a := newVerifyAgent(t, srv, "false") // "false" always fails

	reason, err := a.Run(context.Background(), "x")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Reason must stay StopModelDone (not promoted) since verify failed.
	if reason != StopModelDone {
		t.Errorf("reason = %v, want StopModelDone when verify hook fails at stop time", reason)
	}
	// Feedback must be present in a.Messages.
	found := false
	for _, m := range a.Messages {
		if m.Role != "user" {
			continue
		}
		for _, b := range m.Blocks {
			if tb, ok := b.(llm.TextBlock); ok && strings.Contains(tb.Text, "Verification failed") {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected verify failure feedback in a.Messages at stop time, but none found; messages=%+v", a.Messages)
	}
}
