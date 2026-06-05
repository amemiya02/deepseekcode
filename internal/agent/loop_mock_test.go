package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/llmtest"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// These tests drive the real agent loop end-to-end against the scripted
// offline mock DeepSeek server (internal/llmtest), with no network or
// credentials. They pin the DeepSeek-specific paths that are easy to break
// and expensive to catch live: the finish-reason override, tool-call
// pairing on replay, the thinking-as-struct wire contract, and the two-tier
// stream timeout.

// loopEchoTool records how many times it executed, so a test can prove the
// loop actually dispatched a scripted tool call.
type loopEchoTool struct{ calls *int32 }

func (loopEchoTool) Name() string        { return "echo" }
func (loopEchoTool) Description() string { return "echoes its arguments back" }
func (loopEchoTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}}}`)
}
func (e loopEchoTool) Execute(_ context.Context, args json.RawMessage) (tools.Result, error) {
	atomic.AddInt32(e.calls, 1)
	return tools.Result{Content: "echoed:" + string(args)}, nil
}

// newMockLoopAgent wires an Agent to a scripted mock server with every tool
// auto-approved, a generous step deadline, and a deterministic model id.
func newMockLoopAgent(t *testing.T, srv *llmtest.Server) *Agent {
	t.Helper()
	reg := tools.New()
	pol := permissions.New(permissions.ModeYolo, t.TempDir(), nil, nil, nil)
	a := New(srv.Client(), reg, pol, "deepseek-v4-flash")
	a.System = "test system"
	a.StepTimeout = 10 * time.Second
	return a
}

// TestLoopFinishReasonOverrideContinuesAndPairsResult pins the override from
// docs/design.md §6.4: even when finish_reason=="stop", a non-empty
// tool_calls slice keeps the loop alive, the tool runs, and the next request
// carries the assistant tool_call paired with its tool result.
func TestLoopFinishReasonOverrideContinuesAndPairsResult(t *testing.T) {
	srv := llmtest.NewServer(
		llmtest.Turn{
			Reasoning: "I'll echo it",
			ToolCalls: []llmtest.ToolCall{{ID: "call_1", Name: "echo", Args: `{"text":"hi"}`}},
			Finish:    "stop", // the override: stop + tool_calls must still loop
		},
		llmtest.Turn{Text: "done"},
	)
	defer srv.Close()

	var calls int32
	a := newMockLoopAgent(t, srv)
	a.Tools.Register(loopEchoTool{calls: &calls})

	reason, err := a.Run(context.Background(), "echo hi")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if reason != StopModelDone {
		t.Fatalf("reason = %v, want StopModelDone", reason)
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("echo executed %d times, want 1 — finish-reason override should dispatch the tool", n)
	}
	if srv.Count() != 2 {
		t.Errorf("served %d requests, want 2 — override must force a second turn despite finish_reason=stop", srv.Count())
	}
	assertToolResultPaired(t, srv.Requests()[1], "call_1")
}

// TestLoopThinkingSerializesAsStruct is the end-to-end counterpart to the
// llm-package thinking_shape_test: it pins that the live loop sends
// `thinking` as a struct (not the bool DeepSeek V4 rejects) when enabled,
// and omits it entirely when disabled.
func TestLoopThinkingSerializesAsStruct(t *testing.T) {
	t.Run("enabled", func(t *testing.T) {
		srv := llmtest.NewServer(llmtest.Turn{Text: "ok"})
		defer srv.Close()
		a := newMockLoopAgent(t, srv)
		a.Thinking = true
		a.AutoReasoning = false
		if _, err := a.Run(context.Background(), "implement the parser"); err != nil {
			t.Fatalf("Run: %v", err)
		}
		body := string(srv.LastRequest())
		if !strings.Contains(body, `"thinking":{"type":"enabled"}`) {
			t.Errorf("thinking not serialized as a struct; body=%s", body)
		}
		if strings.Contains(body, `"thinking":true`) {
			t.Error(`thinking serialized as bool "thinking":true — DeepSeek V4 rejects this`)
		}
	})

	t.Run("disabled", func(t *testing.T) {
		srv := llmtest.NewServer(llmtest.Turn{Text: "ok"})
		defer srv.Close()
		a := newMockLoopAgent(t, srv)
		a.Thinking = false
		a.AutoReasoning = false
		if _, err := a.Run(context.Background(), "x"); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if body := string(srv.LastRequest()); strings.Contains(body, `"thinking"`) {
			t.Errorf("thinking field present when disabled (should be omitted); body=%s", body)
		}
	})
}

func TestLoopReasoningEffortAppearsWhenThinkingEnabled(t *testing.T) {
	srv := llmtest.NewServer(llmtest.Turn{Text: "ok"})
	defer srv.Close()
	a := newMockLoopAgent(t, srv)
	a.Thinking = true
	a.AutoReasoning = false
	a.ReasoningEffort = llm.ReasoningEffortMax
	if _, err := a.Run(context.Background(), "implement the parser"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	body := string(srv.LastRequest())
	if !strings.Contains(body, `"reasoning_effort":"max"`) {
		t.Errorf("reasoning_effort:max missing from request; body=%s", body)
	}
}

func TestLoopReasoningEffortOmittedWhenThinkingDisabled(t *testing.T) {
	srv := llmtest.NewServer(llmtest.Turn{Text: "ok"})
	defer srv.Close()
	a := newMockLoopAgent(t, srv)
	a.Thinking = false
	a.AutoReasoning = false
	a.ReasoningEffort = llm.ReasoningEffortMax // set but thinking is off
	if _, err := a.Run(context.Background(), "x"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	body := string(srv.LastRequest())
	if strings.Contains(body, `"reasoning_effort"`) {
		t.Errorf("reasoning_effort should be omitted when thinking disabled; body=%s", body)
	}
}

func TestLoopReasoningEffortDefaultsMaxWhenThinkingEnabled(t *testing.T) {
	srv := llmtest.NewServer(llmtest.Turn{Text: "ok"})
	defer srv.Close()
	a := newMockLoopAgent(t, srv)
	a.Thinking = true
	a.AutoReasoning = false
	// ReasoningEffort left as zero value (empty) — effectiveReasoningEffort
	// should fall back to max when thinking is enabled.
	if _, err := a.Run(context.Background(), "implement the parser"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	body := string(srv.LastRequest())
	if !strings.Contains(body, `"reasoning_effort":"max"`) {
		t.Errorf("reasoning_effort should default to max when thinking enabled; body=%s", body)
	}
}

// TestParityScenario_finish_stop_with_tool_calls pins the agent-loop
// behavior: when finish_reason is "stop" but tool_calls is non-empty,
// the loop must execute the tool call instead of stopping. This is the
// agent-level counterpart to the wire-format parity scenario.
func TestParityScenario_finish_stop_with_tool_calls(t *testing.T) {
	srv := llmtest.NewServer(
		llmtest.Turn{
			Reasoning: "I'll echo it",
			ToolCalls: []llmtest.ToolCall{{ID: "call_1", Name: "echo", Args: `{"text":"hi"}`}},
			Finish:    "stop",
		},
		llmtest.Turn{Text: "done"},
	)
	defer srv.Close()

	var calls int32
	a := newMockLoopAgent(t, srv)
	a.Tools.Register(loopEchoTool{calls: &calls})

	reason, err := a.Run(context.Background(), "echo hi")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if reason != StopModelDone {
		t.Fatalf("reason = %v, want StopModelDone", reason)
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("echo executed %d times, want 1", n)
	}
}

// TestParityScenario_truncated_tool_args_repair pins that malformed or
// truncated JSON tool arguments do not crash the loop — the repair
// system either fixes the args or produces a model-visible error.
func TestParityScenario_truncated_tool_args_repair(t *testing.T) {
	srv := llmtest.NewServer(
		llmtest.Turn{
			ToolCalls: []llmtest.ToolCall{{ID: "call_1", Name: "echo", Args: `{"text":"hi"`}}, // missing closing }
			Finish:    "stop",
		},
		llmtest.Turn{Text: "done"},
	)
	defer srv.Close()

	var calls int32
	a := newMockLoopAgent(t, srv)
	a.Tools.Register(loopEchoTool{calls: &calls})

	// Must not panic or hang — the loop should complete.
	reason, err := a.Run(context.Background(), "echo hi")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if reason != StopModelDone {
		t.Fatalf("reason = %v, want StopModelDone", reason)
	}
}

// TestLoopFirstTokenTimeout pins that a first-token stall that persists across
// the one bounded re-issue (T1.4) surfaces as a first-token timeout the loop
// reports (not a silent hang). The first stall is re-issued; the second is
// fatal, so the error appears only after exactly two requests.
func TestLoopFirstTokenTimeout(t *testing.T) {
	// Both turns effectively never send a first chunk, so the first-token timer
	// is guaranteed to fire first on each attempt regardless of scheduler load.
	srv := llmtest.NewServer(
		llmtest.Turn{DelayFirst: time.Second, Text: "never arrives"},
		llmtest.Turn{DelayFirst: time.Second, Text: "never arrives either"},
	)
	defer srv.Close()

	a := newMockLoopAgent(t, srv)
	a.Client.FirstTokenTimeout = 50 * time.Millisecond
	a.Client.ChunkStallTimeout = 5 * time.Second

	reason, err := a.Run(context.Background(), "x")
	if err == nil {
		t.Fatal("expected a first-token timeout error after the re-issue was exhausted")
	}
	if reason != StopUnknown {
		t.Errorf("reason = %v, want StopUnknown", reason)
	}
	if !strings.Contains(err.Error(), "first-token timeout") {
		t.Errorf("error %q is not a first-token timeout", err)
	}
	if srv.Count() != 2 {
		t.Errorf("served %d requests, want 2 (original + one bounded re-issue)", srv.Count())
	}
}

// TestLoopChunkStallTimeout pins the second tier: once streaming has begun, a
// gap between chunks longer than ChunkStallTimeout aborts the turn. As with the
// first-token tier, T1.4 re-issues once, so the error surfaces only after both
// the original and the re-issue stall.
func TestLoopChunkStallTimeout(t *testing.T) {
	// Reasoning arrives immediately on each turn (starting the stall timer),
	// then the next chunk never comes within the window, so the stall timer
	// always wins.
	srv := llmtest.NewServer(
		llmtest.Turn{Reasoning: "thinking", DelayChunk: time.Second, Text: "slow"},
		llmtest.Turn{Reasoning: "thinking again", DelayChunk: time.Second, Text: "still slow"},
	)
	defer srv.Close()

	a := newMockLoopAgent(t, srv)
	a.Client.FirstTokenTimeout = 5 * time.Second
	a.Client.ChunkStallTimeout = 50 * time.Millisecond

	_, err := a.Run(context.Background(), "x")
	if err == nil {
		t.Fatal("expected a chunk-stall timeout error after the re-issue was exhausted")
	}
	if !strings.Contains(err.Error(), "chunk stall") {
		t.Errorf("error %q is not a chunk-stall timeout", err)
	}
	if srv.Count() != 2 {
		t.Errorf("served %d requests, want 2 (original + one bounded re-issue)", srv.Count())
	}
}

// TestLoopReissuesOnceThenRecovers pins the T1.4 happy path: a single transient
// mid-stream stall is cleared by one re-issue of the identical request, the
// turn completes, and exactly one (non-duplicated) assistant turn is recorded —
// the stalled partial is discarded, not persisted alongside the recovered turn.
func TestLoopReissuesOnceThenRecovers(t *testing.T) {
	srv := llmtest.NewServer(
		// Turn 1 stalls after some reasoning (chunk-stall); turn 2 succeeds.
		llmtest.Turn{Reasoning: "thinking", DelayChunk: time.Second, Text: "stalled"},
		llmtest.Turn{Text: "recovered answer"},
	)
	defer srv.Close()

	a := newMockLoopAgent(t, srv)
	a.Client.FirstTokenTimeout = 5 * time.Second
	a.Client.ChunkStallTimeout = 50 * time.Millisecond
	a.StepTimeout = 0 // don't let a step deadline pre-empt the re-issue

	reason, err := a.Run(context.Background(), "x")
	if err != nil {
		t.Fatalf("Run: %v (a single stall should be recovered by one re-issue)", err)
	}
	if reason != StopModelDone {
		t.Fatalf("reason = %v, want StopModelDone", reason)
	}
	if srv.Count() != 2 {
		t.Fatalf("served %d requests, want 2 (one stall + one re-issue)", srv.Count())
	}

	var assistant []llm.Message
	for _, m := range a.Messages {
		if m.Role == "assistant" {
			assistant = append(assistant, m)
		}
	}
	if len(assistant) != 1 {
		t.Fatalf("recorded %d assistant turns, want exactly 1 — the stalled partial must not be persisted alongside the recovered turn", len(assistant))
	}
	var gotText string
	for _, b := range assistant[0].Blocks {
		if tb, ok := b.(llm.TextBlock); ok {
			gotText = tb.Text
		}
	}
	if !strings.Contains(gotText, "recovered answer") {
		t.Errorf("assistant turn = %q, want the recovered answer (not the stalled partial)", gotText)
	}
}

// TestLoopReissuesAtMostOnce pins the bound: after one re-issue the loop gives
// up and salvages the partial from the final attempt (T1.1), rather than
// re-issuing indefinitely. Exactly two requests are served.
func TestLoopReissuesAtMostOnce(t *testing.T) {
	srv := llmtest.NewServer(
		llmtest.Turn{Reasoning: "first try", DelayChunk: time.Second, Text: "x"},
		llmtest.Turn{Reasoning: "second try", DelayChunk: time.Second, Text: "y"},
		// A third turn exists; if the loop wrongly re-issued again it would be
		// served and the run would succeed, failing the assertions below.
		llmtest.Turn{Text: "should never be reached"},
	)
	defer srv.Close()

	a := newMockLoopAgent(t, srv)
	a.Client.FirstTokenTimeout = 5 * time.Second
	a.Client.ChunkStallTimeout = 50 * time.Millisecond
	a.StepTimeout = 0

	_, err := a.Run(context.Background(), "x")
	if err == nil {
		t.Fatal("expected an error after the single re-issue was exhausted")
	}
	if srv.Count() != 2 {
		t.Fatalf("served %d requests, want exactly 2 (original + one re-issue, no more)", srv.Count())
	}
	// The partial salvaged on give-up is the SECOND attempt's reasoning.
	if len(a.Messages) == 0 {
		t.Fatal("no messages recorded")
	}
	last := a.Messages[len(a.Messages)-1]
	if last.Role != "assistant" {
		t.Fatalf("last message role = %q, want the salvaged partial assistant turn", last.Role)
	}
	var gotThinking string
	for _, b := range last.Blocks {
		if tb, ok := b.(llm.ThinkingBlock); ok {
			gotThinking = tb.Text
		}
	}
	if !strings.Contains(gotThinking, "second try") {
		t.Errorf("salvaged partial reasoning = %q, want the final attempt's reasoning", gotThinking)
	}
}

// TestLoopContextOverflowRoutesToCompaction pins T1.4's other half: a 400
// context-overflow is not retried blindly — the loop compacts and re-attempts
// the step exactly once. The second request (after compaction) succeeds.
func TestLoopContextOverflowRoutesToCompaction(t *testing.T) {
	srv := llmtest.NewServer(
		llmtest.Turn{Status: 400, Body: "This model's maximum context length is 1000000 tokens. However, you requested 1200000 tokens."},
		llmtest.Turn{Text: "ok after compaction"},
	)
	defer srv.Close()

	a := newMockLoopAgent(t, srv)

	// Capture EventInfo so we can assert the overflow was routed to compaction
	// rather than surfaced as a hard error.
	sub := a.Bus().Subscribe(64)
	var sawOverflowNotice atomic.Bool
	go func() {
		for env := range sub.C {
			if info, ok := env.Event.(EventInfo); ok && strings.Contains(info.Text, "context overflow") {
				sawOverflowNotice.Store(true)
			}
		}
	}()

	reason, err := a.Run(context.Background(), "x")
	if err != nil {
		t.Fatalf("Run: %v (overflow should route to compaction, not fail)", err)
	}
	if reason != StopModelDone {
		t.Fatalf("reason = %v, want StopModelDone", reason)
	}
	if srv.Count() != 2 {
		t.Fatalf("served %d requests, want 2 (overflow + one compacted re-attempt)", srv.Count())
	}
	if !sawOverflowNotice.Load() {
		t.Error("expected a 'context overflow' notice signalling the compaction route")
	}
}

// TestLoopAPIErrorPropagates pins that a non-2xx response (retries disabled)
// surfaces as an error from Run rather than a silent stop.
func TestLoopAPIErrorPropagates(t *testing.T) {
	srv := llmtest.NewServer(llmtest.Turn{Status: 500, Body: "kaboom"})
	defer srv.Close()

	a := newMockLoopAgent(t, srv)
	reason, err := a.Run(context.Background(), "x")
	if err == nil {
		t.Fatal("expected an error from a 500 response")
	}
	if reason != StopUnknown {
		t.Errorf("reason = %v, want StopUnknown", reason)
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error %q does not mention status 500", err)
	}
}

// TestLoopPartialPersistOnMidStreamError pins T1.1: when a stream breaks
// mid-turn after some reasoning/text has arrived, that partial assistant
// turn is appended to the message history (and persisted) rather than
// discarded, while the turn still surfaces the error.
func TestLoopPartialPersistOnMidStreamError(t *testing.T) {
	srv := llmtest.NewServer(llmtest.Turn{
		Reasoning:     "partial reasoning",
		Text:          "partial answer so far",
		MalformedTail: true, // content arrives, then the stream breaks
	})
	defer srv.Close()

	a := newMockLoopAgent(t, srv)

	_, err := a.Run(context.Background(), "x")
	if err == nil {
		t.Fatal("expected a mid-stream error to surface from Run")
	}

	if len(a.Messages) == 0 {
		t.Fatal("no messages recorded")
	}
	last := a.Messages[len(a.Messages)-1]
	if last.Role != "assistant" {
		t.Fatalf("last message role = %q, want assistant (partial turn should be appended)", last.Role)
	}
	var gotText, gotThinking string
	for _, b := range last.Blocks {
		switch tb := b.(type) {
		case llm.TextBlock:
			gotText = tb.Text
		case llm.ThinkingBlock:
			gotThinking = tb.Text
		}
	}
	if !strings.Contains(gotText, "partial answer so far") {
		t.Errorf("partial text not preserved; got %q", gotText)
	}
	if !strings.Contains(gotThinking, "partial reasoning") {
		t.Errorf("partial reasoning not preserved; got %q", gotThinking)
	}
}

// TestLoopFirstTokenTimeoutAppendsNothing pins the complement: when the stream
// fails before any content (first-token timeout) on both the original attempt
// and the re-issue, there is nothing to salvage, so no partial assistant turn
// is appended.
func TestLoopFirstTokenTimeoutAppendsNothing(t *testing.T) {
	srv := llmtest.NewServer(
		llmtest.Turn{DelayFirst: time.Second, Text: "never arrives"},
		llmtest.Turn{DelayFirst: time.Second, Text: "never arrives either"},
	)
	defer srv.Close()

	a := newMockLoopAgent(t, srv)
	a.Client.FirstTokenTimeout = 50 * time.Millisecond
	a.Client.ChunkStallTimeout = 5 * time.Second

	if _, err := a.Run(context.Background(), "x"); err == nil {
		t.Fatal("expected a first-token timeout error after the re-issue was exhausted")
	}
	for _, m := range a.Messages {
		if m.Role == "assistant" {
			t.Fatalf("no content arrived, but an assistant message was appended: %+v", m)
		}
	}
}

// TestLoopStepTimeoutIsNonSuccess pins T1.3: a per-step deadline produces a
// typed StopStepTimeout with a non-nil error, not the old StopUnknown,nil
// silent-success shape that rendered a timeout as a clean finish.
func TestLoopStepTimeoutIsNonSuccess(t *testing.T) {
	// The mock blocks before sending headers, so the per-step deadline fires
	// while the client is still in its HTTP round-trip — a reliable
	// connect-phase abort, not a flaky mid-body context cancellation.
	srv := llmtest.NewServer(llmtest.Turn{DelayBeforeHeaders: time.Second, Text: "never arrives"})
	defer srv.Close()

	a := newMockLoopAgent(t, srv)
	a.StepTimeout = 100 * time.Millisecond

	reason, err := a.Run(context.Background(), "x")
	if reason != StopStepTimeout {
		t.Fatalf("reason = %v (err=%v), want StopStepTimeout", reason, err)
	}
	if reason.IsSuccess() {
		t.Error("StopStepTimeout must not report IsSuccess")
	}
	if err == nil {
		t.Fatal("step timeout must surface a non-nil error, not a silent success")
	}
	if !strings.Contains(err.Error(), "step timed out") {
		t.Errorf("error %q does not describe a step timeout", err)
	}
}

// TestLoopUserRequestedStop pins that an explicit RequestStop before
// cancellation is reported as StopUserRequested, distinct from an ambient
// StopContextCancel.
func TestLoopUserRequestedStop(t *testing.T) {
	srv := llmtest.NewServer(llmtest.Turn{DelayBeforeHeaders: time.Second, Text: "never gets here"})
	defer srv.Close()

	a := newMockLoopAgent(t, srv)
	a.StepTimeout = 0 // no per-step deadline; only our cancel drives termination

	ctx, cancel := context.WithCancel(context.Background())
	type res struct {
		reason StopReason
		err    error
	}
	done := make(chan res, 1)
	go func() {
		r, e := a.Run(ctx, "x")
		done <- res{r, e}
	}()

	time.Sleep(40 * time.Millisecond) // let the run reach the in-flight stream
	a.RequestStop()
	cancel()

	select {
	case got := <-done:
		if got.reason != StopUserRequested {
			t.Fatalf("reason = %v, want StopUserRequested", got.reason)
		}
		if got.reason.IsSuccess() {
			t.Error("StopUserRequested must not report IsSuccess")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

// TestStopReasonIsSuccess pins that only model-completed runs are a success.
func TestStopReasonIsSuccess(t *testing.T) {
	cases := []struct {
		r    StopReason
		want bool
	}{
		{StopModelDone, true},
		{StopVerifiedDone, true},   // NEW — must be true
		{StopMaxSteps, false},
		{StopLoopDetected, false},
		{StopContextCancel, false},
		{StopUserRequested, false},
		{StopStepTimeout, false},
		{StopUnknown, false},
	}
	for _, tc := range cases {
		if got := tc.r.IsSuccess(); got != tc.want {
			t.Errorf("StopReason(%d).IsSuccess() = %v, want %v", tc.r, got, tc.want)
		}
	}
}

// assertToolResultPaired checks that a replayed request body carries both the
// assistant tool_call with the given id and its paired tool result.
func assertToolResultPaired(t *testing.T, body []byte, callID string) {
	t.Helper()
	var req struct {
		Messages []struct {
			Role       string `json:"role"`
			ToolCallID string `json:"tool_call_id"`
			ToolCalls  []struct {
				ID string `json:"id"`
			} `json:"tool_calls"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	var sawCall, sawResult bool
	for _, m := range req.Messages {
		for _, tc := range m.ToolCalls {
			if tc.ID == callID {
				sawCall = true
			}
		}
		if m.Role == "tool" && m.ToolCallID == callID {
			sawResult = true
		}
	}
	if !sawCall {
		t.Errorf("assistant tool_call %q absent from replayed request", callID)
	}
	if !sawResult {
		t.Errorf("paired tool result for %q absent from replayed request", callID)
	}
}
