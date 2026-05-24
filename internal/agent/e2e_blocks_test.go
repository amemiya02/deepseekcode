package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/session"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// echoTool is a minimal Tool that returns a fixed string. Lives only
// in this test file so changes to the real registry can't perturb
// the e2e contract.
type echoTool struct{}

func (echoTool) Name() string                { return "echo" }
func (echoTool) Description() string         { return "echoes a fixed string" }
func (echoTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (echoTool) Execute(_ context.Context, _ json.RawMessage) (tools.Result, error) {
	return tools.Result{Content: "echoed"}, nil
}

// TestE2EBlocksRoundtrip pins the Phase 0 contract end-to-end:
// user input → agent step (Thinking + Text + ToolUse) → tool execution
// (ToolResult) → next step (Text) → persistence → reload → identical
// wire bytes.
func TestE2EBlocksRoundtrip(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		switch n {
		case 1:
			// Reasoning + tool_call. finish_reason=tool_calls keeps the loop alive.
			emitSSE(w,
				`{"choices":[{"index":0,"delta":{"reasoning_content":"plan"}}]}`,
				`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"echo","arguments":"{}"}}]}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`,
			)
		case 2:
			// Final assistant text + stop.
			emitSSE(w,
				`{"choices":[{"index":0,"delta":{"content":"all done"}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":15,"completion_tokens":3,"total_tokens":18}}`,
			)
		default:
			http.Error(w, "unexpected extra call", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	store, err := session.Open(filepath.Join(t.TempDir(), "e2e.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	sess, err := store.NewSession(context.Background(), "/proj", "test-model", false)
	if err != nil {
		t.Fatalf("new session: %v", err)
	}

	client := llm.NewClient("k", srv.URL)
	reg := tools.New()
	reg.Register(echoTool{})
	pol := permissions.New(permissions.ModeYolo, "", nil, nil, nil)
	a := New(client, reg, pol, "test-model")
	a.System = "sys"
	a.Persister = session.NewPersister(store, nil, sess.ID)
	a.StopWhen = []StopCondition{MaxSteps(5)}

	if _, err := a.Run(context.Background(), "hello"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Capture wire bytes from the in-memory message slice.
	wire1, err := buildRequest(a).MarshalCacheStable()
	if err != nil {
		t.Fatalf("marshal in-memory: %v", err)
	}

	// Re-load from store and rebuild the message slice.
	loaded, err := store.LoadMessages(context.Background(), sess.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got, want := len(loaded), 4; got != want {
		t.Fatalf("loaded msg count: got %d want %d", got, want)
	}
	assertKinds(t, "user", loaded[0].Blocks, []llm.BlockKind{llm.BlockKindText})
	assertKinds(t, "assistant#1", loaded[1].Blocks, []llm.BlockKind{
		llm.BlockKindThinking, llm.BlockKindToolUse,
	})
	assertKinds(t, "tool", loaded[2].Blocks, []llm.BlockKind{llm.BlockKindToolResult})
	assertKinds(t, "assistant#2", loaded[3].Blocks, []llm.BlockKind{llm.BlockKindText})

	a2 := New(client, reg, pol, "test-model")
	a2.System = "sys"
	for _, m := range loaded {
		a2.Messages = append(a2.Messages, llm.Message{Role: m.Role, Blocks: m.Blocks})
	}
	wire2, err := buildRequest(a2).MarshalCacheStable()
	if err != nil {
		t.Fatalf("marshal reloaded: %v", err)
	}
	if !bytes.Equal(wire1, wire2) {
		t.Errorf("wire bytes diverge after reload:\nin-memory: %s\nreloaded:  %s", wire1, wire2)
	}
}

func buildRequest(a *Agent) llm.Request {
	return llm.Request{
		Model:    a.Model,
		Messages: a.fullMessages(),
		Tools:    a.Tools.AsLLMTools(),
		Thinking: llm.ThinkingEnabled(a.Thinking),
	}
}

func emitSSE(w http.ResponseWriter, frames ...string) {
	for _, f := range frames {
		fmt.Fprintf(w, "data: %s\n\n", f)
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func assertKinds(t *testing.T, label string, blocks []llm.ContentBlock, want []llm.BlockKind) {
	t.Helper()
	if len(blocks) != len(want) {
		t.Errorf("%s: kind count: got %d want %d (%#v)", label, len(blocks), len(want), blocks)
		return
	}
	for i, b := range blocks {
		if b.BlockKind() != want[i] {
			t.Errorf("%s[%d]: got %s want %s", label, i, b.BlockKind(), want[i])
		}
	}
}
