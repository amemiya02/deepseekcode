package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// TestE2ERulesDenyTool validates that a deny rule:
//  1. Emits an EventInfo with "denied by rule" reason.
//  2. Produces a tool error (ToolResultBlock.IsError=true) containing
//     "denied by rule".
func TestE2ERulesDenyTool(t *testing.T) {
	dir := t.TempDir()

	engine := &permissions.RuleEngine{
		Deny: []permissions.PermissionRule{
			{ToolPattern: "bash", ArgsPattern: "", Decision: "deny"},
		},
	}

	pol := permissions.New(permissions.ModeDefault, dir, nil, nil, engine)
	reg := tools.New()
	tools.RegisterBuiltins(reg, 0, 0, dir)

	argsJSON, _ := json.Marshal(map[string]any{"command": "echo hello"})
	argsStr, _ := json.Marshal(string(argsJSON))

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		switch n {
		case 1:
			emitSSE(w,
				`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"bash","arguments":`+string(argsStr)+`}}]}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`,
			)
		case 2:
			emitSSE(w,
				`{"choices":[{"index":0,"delta":{"content":"done"}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":15,"completion_tokens":1,"total_tokens":16}}`,
			)
		default:
			http.Error(w, "unexpected call", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	a := New(llm.NewClient("k", srv.URL), reg, pol, "test-model")
	a.StopWhen = []StopCondition{MaxSteps(2)}

	_, err := a.Run(context.Background(), "run bash")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Drain the buffered events channel without blocking on close.
	var collected []Event
drain:
	for {
		select {
		case ev := <-a.Events():
			collected = append(collected, ev)
		default:
			break drain
		}
	}

	// 1. Verify EventInfo emitted with "denied by rule".
	infoFound := false
	for _, ev := range collected {
		if info, ok := ev.(EventInfo); ok {
			if strings.Contains(info.Text, "denied by rule") {
				infoFound = true
			}
		}
	}
	if !infoFound {
		t.Error("expected EventInfo with 'denied by rule', none found")
	}

	// 2. Verify ToolResultBlock with IsError and "denied by rule".
	blockFound := false
	for _, msg := range a.Messages {
		if msg.Role == "tool" {
			for _, b := range msg.Blocks {
				if tb, ok := b.(llm.ToolResultBlock); ok {
					if tb.IsError && strings.Contains(tb.Content, "denied by rule") {
						blockFound = true
					}
				}
			}
		}
	}
	if !blockFound {
		t.Error("expected ToolResultBlock with IsError and 'denied by rule', none found")
	}
}
