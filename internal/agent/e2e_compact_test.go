package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// TestE2ECompactAcrossTurns runs the agent through enough tool-bearing
// turns that compaction must fire at least once with a low threshold.
// Afterward every remaining ToolUseBlock must still have a matching
// ToolResultBlock — the load-bearing invariant adjustBoundary
// protects.
func TestE2ECompactAcrossTurns(t *testing.T) {
	const totalTurns = 12
	const finalTurn = totalTurns

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Pad the assistant text so estimated tokens climb quickly
		// enough to cross the (deliberately low) threshold.
		pad := strings.Repeat("p", 600)
		if int(n) < finalTurn {
			emitSSE(w,
				fmt.Sprintf(`{"choices":[{"index":0,"delta":{"content":%q}}]}`, pad),
				fmt.Sprintf(`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c%d","type":"function","function":{"name":"echo","arguments":"{}"}}]}}]}`, n),
				`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":5,"total_tokens":10}}`,
			)
			return
		}
		emitSSE(w,
			`{"choices":[{"index":0,"delta":{"content":"all done"}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`,
		)
	}))
	defer srv.Close()

	client := llm.NewClient("k", srv.URL)
	reg := tools.New()
	reg.Register(echoTool{})
	a := New(client, reg, permissions.New(permissions.ModeYolo, "", nil, nil, nil), "m")
	a.System = "sys"
	a.CompactionCfg = CompactionConfig{
		PreserveRecentMessages: 4,
		AutoCompactInputTokens: 200, // low enough to fire after a few turns
	}
	a.StopWhen = []StopCondition{MaxSteps(totalTurns + 1)}

	if _, err := a.Run(context.Background(), "kick off"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	compactionCount := 0
DRAIN:
	for {
		select {
		case ev := <-a.Events():
			if _, ok := ev.(EventCompaction); ok {
				compactionCount++
			}
		default:
			break DRAIN
		}
	}
	if compactionCount < 1 {
		t.Errorf("expected at least one compaction; saw %d", compactionCount)
	}

	// Pairing invariant: every ToolUseBlock in the surviving message
	// list must have a corresponding ToolResultBlock.
	uses := map[string]bool{}
	results := map[string]bool{}
	for _, m := range a.Messages {
		for _, b := range m.Blocks {
			switch v := b.(type) {
			case llm.ToolUseBlock:
				uses[v.ID] = true
			case llm.ToolResultBlock:
				results[v.ToolUseID] = true
			}
		}
	}
	for id := range uses {
		if !results[id] {
			t.Errorf("ToolUseID %q has no matching ToolResultBlock post-compaction", id)
		}
	}
}
