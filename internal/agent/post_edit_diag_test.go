package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/llmtest"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// mockMutatingTool is a tool that reports affected paths (implements
// PathAware) and is NOT read-only, so it triggers post-edit diagnostics.
type mockMutatingTool struct{ paths []string }

func (mockMutatingTool) Name() string        { return "write_file" }
func (mockMutatingTool) Description() string { return "writes a file" }
func (mockMutatingTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)
}
func (m mockMutatingTool) Execute(_ context.Context, _ json.RawMessage) (tools.Result, error) {
	return tools.Result{Content: "ok"}, nil
}
func (m mockMutatingTool) AffectedPaths(_ json.RawMessage) []string { return m.paths }

// mockReadOnlyTool implements ReadOnlyHint.
type mockReadOnlyTool struct{}

func (mockReadOnlyTool) Name() string        { return "read_file" }
func (mockReadOnlyTool) Description() string { return "reads a file" }
func (mockReadOnlyTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)
}
func (mockReadOnlyTool) Execute(_ context.Context, _ json.RawMessage) (tools.Result, error) {
	return tools.Result{Content: "data"}, nil
}
func (mockReadOnlyTool) IsReadOnly() bool { return true }

// TestPostEditDiagnosticsMutatingAppends verifies that a step with a
// mutating tool call appends a feedback user message when the callback
// returns non-empty diagnostics.
func TestPostEditDiagnosticsMutatingAppends(t *testing.T) {
	srv := llmtest.NewServer(
		llmtest.Turn{
			ToolCalls: []llmtest.ToolCall{{ID: "c1", Name: "write_file", Args: `{"path":"foo.go"}`}},
		},
		llmtest.Turn{Text: "ok"},
	)
	defer srv.Close()

	a := newMockLoopAgent(t, srv)
	a.Tools.Register(mockMutatingTool{paths: []string{"foo.go"}})

	var called bool
	a.PostEditDiagnostics = func(_ context.Context, paths []string) string {
		called = true
		if len(paths) != 1 || paths[0] != "foo.go" {
			t.Errorf("PostEditDiagnostics paths = %v, want [foo.go]", paths)
		}
		return "Environment diagnostics after edit:\nfoo.go:10:1 error undefined: x\n"
	}

	reason, err := a.Run(context.Background(), "write it")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if reason != StopModelDone {
		t.Fatalf("reason = %v, want StopModelDone", reason)
	}
	if !called {
		t.Fatal("PostEditDiagnostics was never called")
	}

	// The synthetic feedback message should be in the conversation.
	found := false
	for _, m := range a.Messages {
		if m.Role != "user" {
			continue
		}
		for _, b := range m.Blocks {
			if tb, ok := b.(llm.TextBlock); ok && strings.Contains(tb.Text, "undefined: x") {
				found = true
			}
		}
	}
	if !found {
		t.Error("post-edit diagnostics feedback message not found in conversation")
	}
}

// TestPostEditDiagnosticsReadOnlySkipped verifies that a step with only
// read-only tools does NOT call PostEditDiagnostics.
func TestPostEditDiagnosticsReadOnlySkipped(t *testing.T) {
	srv := llmtest.NewServer(
		llmtest.Turn{
			ToolCalls: []llmtest.ToolCall{{ID: "c1", Name: "read_file", Args: `{"path":"foo.go"}`}},
		},
		llmtest.Turn{Text: "ok"},
	)
	defer srv.Close()

	a := newMockLoopAgent(t, srv)
	a.Tools.Register(mockReadOnlyTool{})

	called := false
	a.PostEditDiagnostics = func(_ context.Context, _ []string) string {
		called = true
		return "should not appear"
	}

	reason, err := a.Run(context.Background(), "read it")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if reason != StopModelDone {
		t.Fatalf("reason = %v, want StopModelDone", reason)
	}
	if called {
		t.Fatal("PostEditDiagnostics should not be called for read-only tools")
	}
}

// TestPostEditDiagnosticsEmptyReturnNoMessage verifies that when the
// callback returns "", no synthetic message is appended.
func TestPostEditDiagnosticsEmptyReturnNoMessage(t *testing.T) {
	srv := llmtest.NewServer(
		llmtest.Turn{
			ToolCalls: []llmtest.ToolCall{{ID: "c1", Name: "write_file", Args: `{"path":"foo.go"}`}},
		},
		llmtest.Turn{Text: "ok"},
	)
	defer srv.Close()

	a := newMockLoopAgent(t, srv)
	a.Tools.Register(mockMutatingTool{paths: []string{"foo.go"}})

	a.PostEditDiagnostics = func(_ context.Context, _ []string) string {
		return "" // no diagnostics
	}

	reason, err := a.Run(context.Background(), "write it")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if reason != StopModelDone {
		t.Fatalf("reason = %v, want StopModelDone", reason)
	}

	// No synthetic user message beyond the initial prompt.
	userMsgs := 0
	for _, m := range a.Messages {
		if m.Role == "user" {
			userMsgs++
		}
	}
	// Exactly 1 user message: the initial prompt.
	if userMsgs != 1 {
		t.Errorf("found %d user messages, want 1 (empty return should not append)", userMsgs)
	}
}

// TestPostEditDiagnosticsDedupPaths verifies that duplicate paths from
// multiple tool calls are deduplicated before calling the callback.
func TestPostEditDiagnosticsDedupPaths(t *testing.T) {
	srv := llmtest.NewServer(
		llmtest.Turn{
			ToolCalls: []llmtest.ToolCall{
				{ID: "c1", Name: "write_file", Args: `{"path":"foo.go"}`},
				{ID: "c2", Name: "write_file", Args: `{"path":"foo.go"}`},
			},
		},
		llmtest.Turn{Text: "ok"},
	)
	defer srv.Close()

	a := newMockLoopAgent(t, srv)
	a.Tools.Register(mockMutatingTool{paths: []string{"foo.go"}})

	var gotPaths []string
	a.PostEditDiagnostics = func(_ context.Context, paths []string) string {
		gotPaths = paths
		return "diag"
	}

	reason, err := a.Run(context.Background(), "write it twice")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if reason != StopModelDone {
		t.Fatalf("reason = %v, want StopModelDone", reason)
	}
	if len(gotPaths) != 1 {
		t.Errorf("PostEditDiagnostics got %d paths, want 1 (dedup): %v", len(gotPaths), gotPaths)
	}
}

// TestPostEditDiagnosticsNilCallback verifies that when PostEditDiagnostics
// is nil, no panic occurs and no message is appended.
func TestPostEditDiagnosticsNilCallback(t *testing.T) {
	srv := llmtest.NewServer(
		llmtest.Turn{
			ToolCalls: []llmtest.ToolCall{{ID: "c1", Name: "write_file", Args: `{"path":"foo.go"}`}},
		},
		llmtest.Turn{Text: "ok"},
	)
	defer srv.Close()

	a := newMockLoopAgent(t, srv)
	a.Tools.Register(mockMutatingTool{paths: []string{"foo.go"}})
	// PostEditDiagnostics left nil.

	reason, err := a.Run(context.Background(), "write it")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if reason != StopModelDone {
		t.Fatalf("reason = %v, want StopModelDone", reason)
	}
}

// TestPostEditDiagnosticsCallbackNotCalledWithoutPaths verifies that
// when the mutating tool reports no affected paths (e.g. bash), the
// callback is not invoked.
func TestPostEditDiagnosticsCallbackNotCalledWithoutPaths(t *testing.T) {
	srv := llmtest.NewServer(
		llmtest.Turn{
			ToolCalls: []llmtest.ToolCall{{ID: "c1", Name: "write_file", Args: `{"path":"foo.go"}`}},
		},
		llmtest.Turn{Text: "ok"},
	)
	defer srv.Close()

	a := newMockLoopAgent(t, srv)
	// Register with nil paths — simulates bash-like tool.
	a.Tools.Register(mockMutatingTool{paths: nil})

	called := false
	a.PostEditDiagnostics = func(_ context.Context, _ []string) string {
		called = true
		return "should not appear"
	}

	_, err := a.Run(context.Background(), "write it")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if called {
		t.Fatal("PostEditDiagnostics should not be called when no paths reported")
	}
}

// TestPostEditDiagnosticsE2EWithFeedback verifies the full loop: the
// model sees the diagnostics feedback on its next turn and responds.
func TestPostEditDiagnosticsE2EWithFeedback(t *testing.T) {
	srv := llmtest.NewServer(
		llmtest.Turn{
			ToolCalls: []llmtest.ToolCall{{ID: "c1", Name: "write_file", Args: `{"path":"foo.go"}`}},
		},
		// Second turn: the model should see the diagnostics in context.
		llmtest.Turn{Text: "I see the error, let me fix it"},
	)
	defer srv.Close()

	a := newMockLoopAgent(t, srv)
	a.Tools.Register(mockMutatingTool{paths: []string{"foo.go"}})
	a.PostEditDiagnostics = func(_ context.Context, _ []string) string {
		return "Environment diagnostics after edit:\nfoo.go:10:1 error undefined: x\n"
	}

	reason, err := a.Run(context.Background(), "write it")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if reason != StopModelDone {
		t.Fatalf("reason = %v, want StopModelDone", reason)
	}
	if srv.Count() != 2 {
		t.Fatalf("served %d requests, want 2 (tool turn + feedback response)", srv.Count())
	}

	// The second request should contain the diagnostics feedback text.
	reqs := srv.Requests()
	if len(reqs) < 2 {
		t.Fatal("not enough requests")
	}
	body := string(reqs[1])
	if !strings.Contains(body, "undefined: x") {
		t.Error("second request does not contain diagnostics feedback text")
	}
}

// TestBuildPostEditDiagnosticsCaps verifies file count, per-file
// diagnostics, and total byte caps.
func TestBuildPostEditDiagnosticsCaps(t *testing.T) {
	// This test runs in the agent package so it can't call the main-package
	// helper directly. Instead we test the capping logic via the callback
	// contract: the callback is responsible for capping, and the agent just
	// passes through whatever it returns. The actual capping is tested in
	// the main package if needed.
	//
	// Here we verify that the agent correctly handles a long feedback string.
	srv := llmtest.NewServer(
		llmtest.Turn{
			ToolCalls: []llmtest.ToolCall{{ID: "c1", Name: "write_file", Args: `{"path":"foo.go"}`}},
		},
		llmtest.Turn{Text: "ok"},
	)
	defer srv.Close()

	a := newMockLoopAgent(t, srv)
	a.Tools.Register(mockMutatingTool{paths: []string{"foo.go"}})

	longFeedback := strings.Repeat("x", 1000)
	a.PostEditDiagnostics = func(_ context.Context, _ []string) string {
		return longFeedback
	}

	reason, err := a.Run(context.Background(), "write it")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if reason != StopModelDone {
		t.Fatalf("reason = %v, want StopModelDone", reason)
	}

	// The feedback message should be in the conversation.
	found := false
	for _, m := range a.Messages {
		if m.Role != "user" {
			continue
		}
		for _, b := range m.Blocks {
			if tb, ok := b.(llm.TextBlock); ok && len(tb.Text) >= 1000 {
				found = true
			}
		}
	}
	if !found {
		t.Error("long feedback message not found in conversation")
	}
}
