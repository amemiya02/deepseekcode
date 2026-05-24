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

// TestPhase2FullSmoke is the Phase 2 acceptance smoke: a long
// fake-agent run that triggers at least two automatic compactions,
// then a manual ForceCompact, with the final message list smaller
// than the historical max. ≥3 EventCompaction must be observed.
func TestPhase2FullSmoke(t *testing.T) {
	const totalTurns = 30

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		pad := strings.Repeat("p", 1200)
		if int(n) < totalTurns {
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
	a := New(client, reg, permissions.New(permissions.ModeYolo, "", nil, nil), "m")
	a.System = "sys"
	a.CompactionCfg = CompactionConfig{
		PreserveRecentMessages: 4,
		// Low threshold + 1200-byte pad per turn → compaction every
		// few steps, easily ≥2 over a 30-turn run.
		AutoCompactInputTokens: 200,
	}
	a.StopWhen = []StopCondition{MaxSteps(totalTurns + 1)}

	if _, err := a.Run(context.Background(), "kick off"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Manual force-compact at the tail.
	a.ForceCompact(context.Background())

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
	if compactionCount < 3 {
		t.Errorf("expected ≥3 compactions (auto + auto + manual); saw %d", compactionCount)
	}
	// Uncompacted: each tool-bearing turn adds ~2 messages (assistant
	// + tool result), plus one user kick-off, so a 30-turn run would
	// land near 60 messages without compaction. Compacted: the final
	// list should be well under half that.
	uncompactedUpperBound := totalTurns * 2
	if len(a.Messages) >= uncompactedUpperBound/2 {
		t.Errorf("final message list (%d) should be < half the uncompacted upper bound (%d)",
			len(a.Messages), uncompactedUpperBound)
	}
}
