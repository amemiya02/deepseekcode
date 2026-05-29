package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/llmtest"
)

// T2.1 — loop-detection corrective nudge. These tests drive the real loop
// against the offline mock and pin the contract: the FIRST loop detection in a
// Run injects exactly one synthetic-result-plus-user nudge and continues
// (forgiving the pre-nudge repeats via loopFloor), giving the model one chance
// to break out; a SECOND detection in the same Run hard-stops as before.

// identicalEchoTurn scripts one turn that calls echo with fixed args, so a run
// of them trips the loop detector (window 5, maxRepeats 3) on the third.
func identicalEchoTurn(id string) llmtest.Turn {
	return llmtest.Turn{ToolCalls: []llmtest.ToolCall{{ID: id, Name: "echo", Args: `{"text":"again"}`}}}
}

// TestLoopDetectNudgeContinuesOnce: three identical tool calls trip detection;
// the nudge fires once and the loop continues to a recovery turn that finishes
// cleanly. Pins reason==StopModelDone, exactly one nudge, a valid paired
// transcript on the post-nudge request, and that the cache-stable system
// prefix is byte-identical across the nudge (the nudge is tail-only).
func TestLoopDetectNudgeContinuesOnce(t *testing.T) {
	srv := llmtest.NewServer(
		identicalEchoTurn("call_1"),
		identicalEchoTurn("call_2"),
		identicalEchoTurn("call_3"),                          // third identical → loop detected here
		llmtest.Turn{Text: "ok, trying something different"}, // recovery turn
	)
	defer srv.Close()

	var calls int32
	a := newMockLoopAgent(t, srv)
	a.Tools.Register(loopEchoTool{calls: &calls})

	infos := captureInfo(a)

	reason, err := a.Run(context.Background(), "go")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if reason != StopModelDone {
		t.Fatalf("reason = %v, want StopModelDone (the nudge should let the model recover)", reason)
	}
	if srv.Count() != 4 {
		t.Fatalf("served %d requests, want 4 — the loop must continue past the detection point", srv.Count())
	}

	// Exactly one corrective nudge surfaced.
	nudges := 0
	for _, s := range infos() {
		if strings.Contains(s, "loop detected") {
			nudges++
		}
	}
	if nudges != 1 {
		t.Errorf("emitted %d loop-detection notices, want exactly 1", nudges)
	}

	// Exactly one user-role nudge message landed in the transcript.
	userNudges := 0
	for _, m := range a.Messages {
		if m.Role != "user" {
			continue
		}
		for _, b := range m.Blocks {
			if tb, ok := b.(llm.TextBlock); ok && tb.Text == loopBreakNudgeText {
				userNudges++
			}
		}
	}
	if userNudges != 1 {
		t.Errorf("found %d injected user nudges in a.Messages, want 1", userNudges)
	}

	// The post-nudge request (#4) must carry the dangling tool_call (call_3)
	// paired with its synthetic result — otherwise DeepSeek 400s on the
	// continued turn.
	assertToolResultPaired(t, srv.Requests()[3], "call_3")

	// The synthetic result for the dangling call is an error result with the
	// loop-break text.
	if !transcriptHasErrorToolResult(a.Messages, "call_3", loopBreakToolResultText) {
		t.Errorf("dangling call_3 was not closed with a synthetic error tool-result")
	}

	// Tail-only invariant: the cache-stable system prefix must be byte-identical
	// before and after the nudge (the nudge never touches system/tools).
	if sys1, sys4 := systemMessageOf(t, srv.Requests()[0]), systemMessageOf(t, srv.Requests()[3]); sys1 != sys4 {
		t.Errorf("system prefix changed across the nudge — it must be tail-only\nbefore=%q\nafter=%q", sys1, sys4)
	}
}

// TestLoopDetectHardStopsOnSecondDetection: when the model ignores the nudge
// and keeps repeating, the SECOND detection (now past loopFloor) hard-stops
// with StopLoopDetected, and the nudge fires exactly once total (no nudge-loop).
func TestLoopDetectHardStopsOnSecondDetection(t *testing.T) {
	srv := llmtest.NewServer(
		identicalEchoTurn("call_1"),
		identicalEchoTurn("call_2"),
		identicalEchoTurn("call_3"), // first detection → nudge
		identicalEchoTurn("call_4"),
		identicalEchoTurn("call_5"),
		identicalEchoTurn("call_6"), // second detection (past floor) → hard stop
	)
	defer srv.Close()

	var calls int32
	a := newMockLoopAgent(t, srv)
	a.Tools.Register(loopEchoTool{calls: &calls})

	infos := captureInfo(a)

	reason, err := a.Run(context.Background(), "go")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if reason != StopLoopDetected {
		t.Fatalf("reason = %v, want StopLoopDetected on the second detection", reason)
	}
	if reason.IsSuccess() {
		t.Error("StopLoopDetected must not report IsSuccess")
	}
	if srv.Count() != 6 {
		t.Fatalf("served %d requests, want 6 (3 to first detect + nudge, 3 more to re-detect)", srv.Count())
	}

	nudges := 0
	for _, s := range infos() {
		if strings.Contains(s, "loop detected") {
			nudges++
		}
	}
	if nudges != 1 {
		t.Errorf("emitted %d loop-detection notices, want exactly 1 (no nudge-loop)", nudges)
	}
}

// --- helpers ---

// captureInfo subscribes to the bus and returns an accessor for all EventInfo
// texts seen so far. The subscription drains on its own goroutine; the mutex
// keeps the accessor -race clean against that goroutine.
func captureInfo(a *Agent) func() []string {
	sub := a.Bus().Subscribe(256)
	var (
		mu   sync.Mutex
		seen []string
	)
	go func() {
		for env := range sub.C {
			if info, ok := env.Event.(EventInfo); ok {
				mu.Lock()
				seen = append(seen, info.Text)
				mu.Unlock()
			}
		}
	}()
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), seen...)
	}
}

func transcriptHasErrorToolResult(msgs []llm.Message, callID, content string) bool {
	for _, m := range msgs {
		if m.Role != "tool" {
			continue
		}
		for _, b := range m.Blocks {
			if tr, ok := b.(llm.ToolResultBlock); ok && tr.ToolUseID == callID && tr.IsError && strings.Contains(tr.Content, content) {
				return true
			}
		}
	}
	return false
}

// systemMessageOf extracts the raw system-message content from a wire request
// body. Content is decoded as RawMessage so the comparison is robust whether
// the provider serializes it as a bare string or a content array.
func systemMessageOf(t *testing.T, body []byte) string {
	t.Helper()
	var req struct {
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	for _, m := range req.Messages {
		if m.Role == "system" {
			return string(m.Content)
		}
	}
	return ""
}
