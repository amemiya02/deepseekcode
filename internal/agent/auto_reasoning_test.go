package agent

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

func TestLastUserText(t *testing.T) {
	a := New(nil, tools.New(), nil, "")
	a.Messages = []llm.Message{
		{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "hello"}}},
		{Role: "assistant", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "hi there"}}},
		{Role: "tool", Blocks: []llm.ContentBlock{llm.ToolResultBlock{Content: "ok"}}},
		{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "调试这个崩溃"}}},
	}
	got := a.lastUserText()
	if got != "调试这个崩溃" {
		t.Errorf("lastUserText = %q, want %q", got, "调试这个崩溃")
	}
}

func TestLastUserTextNoUser(t *testing.T) {
	a := New(nil, tools.New(), nil, "")
	a.Messages = []llm.Message{
		{Role: "assistant", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "hello"}}},
		{Role: "tool", Blocks: []llm.ContentBlock{llm.ToolResultBlock{Content: "ok"}}},
	}
	if got := a.lastUserText(); got != "" {
		t.Errorf("lastUserText with no user msg = %q, want empty", got)
	}
}

func TestLastUserTextEmpty(t *testing.T) {
	a := New(nil, tools.New(), nil, "")
	if got := a.lastUserText(); got != "" {
		t.Errorf("lastUserText on empty Messages = %q, want empty", got)
	}
}

func TestLastUserTextMultipleBlocks(t *testing.T) {
	a := New(nil, tools.New(), nil, "")
	a.Messages = []llm.Message{
		{Role: "user", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: "first block"},
			llm.TextBlock{Text: "second block"},
		}},
	}
	got := a.lastUserText()
	if got != "first block" {
		t.Errorf("lastUserText should return first TextBlock, got %q", got)
	}
}

// captureThinking starts a one-step agent run against an httptest server
// that captures the raw request body. Returns true if the captured body
// contains the "thinking" field (i.e. thinking was enabled for this turn).
func captureThinking(t *testing.T, prompt string, auto, thinking bool) bool {
	t.Helper()
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		emitSSE(w,
			`{"choices":[{"index":0,"delta":{"content":"ok"}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"total_tokens":2}}`,
		)
	}))
	defer srv.Close()

	a := New(llm.NewClient("k", srv.URL), tools.New(),
		permissions.New(permissions.ModeYolo, t.TempDir(), nil, nil, nil), "m")
	a.Thinking = thinking
	a.AutoReasoning = auto
	a.StopWhen = []StopCondition{MaxSteps(1)}
	if _, err := a.Run(context.Background(), prompt); err != nil {
		t.Fatal(err)
	}
	return strings.Contains(body, `"thinking"`)
}

func TestRunStepAutoReasoningHigh(t *testing.T) {
	// HIGH keyword "调试" → thinking ON even when default is false
	if !captureThinking(t, "帮我调试这个崩溃", true, false) {
		t.Error("HIGH keyword should enable thinking")
	}
}

func TestRunStepAutoReasoningLow(t *testing.T) {
	// LOW keyword "search" → thinking OFF even when default is true
	if captureThinking(t, "search for the file", true, true) {
		t.Error("LOW keyword should disable thinking")
	}
}

func TestRunStepAutoReasoningOff(t *testing.T) {
	// AutoReasoning=false → preserves the static Thinking field (true)
	if !captureThinking(t, "implement the parser", false, true) {
		t.Error("AutoReasoning=false should preserve Thinking=true")
	}
	// AutoReasoning=false + Thinking=false → no thinking
	if captureThinking(t, "implement the parser", false, false) {
		t.Error("AutoReasoning=false should preserve Thinking=false")
	}
}
