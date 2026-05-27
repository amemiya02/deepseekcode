package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/agents"
	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// TestE2ESubagentChildTrace runs a real parent→subagent loop with a JSONL
// trace sink attached and proves the subagent's epoch is teed into the same
// trace stamped agent_role="subagent", carrying a distinct child epoch_id that
// does not reuse the parent epoch, plus a parent_epoch_id pointing back at the
// actual parent (root) epoch. This is the evidence the cache gate needs to
// judge parent/subagent isolation instead of hardcoding it.
func TestE2ESubagentChildTrace(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		switch n {
		case 1: // Parent turn: call the task tool to spawn a subagent.
			emitSSE(w,
				`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"task","arguments":"{\"description\":\"find bugs\"}"}}]}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`,
			)
		case 2: // Child turn: final assistant text + stop.
			emitSSE(w,
				`{"choices":[{"index":0,"delta":{"content":"found 2 bugs"}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`,
			)
		case 3: // Parent turn after task result: stop.
			emitSSE(w,
				`{"choices":[{"index":0,"delta":{"content":"done"}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":15,"completion_tokens":2,"total_tokens":17}}`,
			)
		default:
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	client := llm.NewClient("k", srv.URL)
	reg := tools.New()
	reg.Register(stubTool{name: "read_file"})
	pol := permissions.New(permissions.ModeYolo, "", nil, nil, nil)
	parent := New(client, reg, pol, "test-model")
	parent.System = "sys"
	parent.StopWhen = []StopCondition{MaxSteps(10)}

	var buf bytes.Buffer
	handle := parent.AttachTraceSink(&buf)

	spawner := &LoopSpawner{Client: client, Parent: parent, Defs: map[string]agents.AgentDef{}}
	reg.Register(tools.NewSubagentTool(spawner))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _, _ = parent.Run(ctx, "use the task tool to find bugs") }()

	for ev := range parent.Events() {
		if _, ok := ev.(EventDone); ok {
			break
		}
	}
	handle.WaitTimeout(2 * time.Second)
	handle.Close()

	type rec struct {
		Type          string `json:"type"`
		EpochID       string `json:"epoch_id"`
		AgentRole     string `json:"agent_role"`
		ParentEpochID string `json:"parent_epoch_id"`
	}
	rootEpochs := map[string]bool{}
	childEpochs := map[string]bool{}
	var childParent string
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var rr rec
		if err := json.Unmarshal([]byte(line), &rr); err != nil {
			t.Fatalf("malformed trace line %q: %v", line, err)
		}
		if rr.EpochID == "" {
			continue
		}
		if rr.AgentRole == "subagent" {
			childEpochs[rr.EpochID] = true
			if rr.ParentEpochID != "" {
				childParent = rr.ParentEpochID
			}
		} else {
			rootEpochs[rr.EpochID] = true
		}
	}

	if len(rootEpochs) == 0 {
		t.Fatal("no root epoch in trace")
	}
	if len(childEpochs) == 0 {
		t.Fatal("no subagent (child) epoch in trace — child trace not wired")
	}
	for ce := range childEpochs {
		if rootEpochs[ce] {
			t.Errorf("child epoch %q also seen as a root epoch — parent/subagent pollution", ce)
		}
	}
	if childParent == "" {
		t.Error("child trace missing parent_epoch_id")
	} else if !rootEpochs[childParent] {
		t.Errorf("child parent_epoch_id %q is not a known root epoch %v", childParent, rootEpochs)
	}
}

// TestWaitChildTracesFlushesAsyncChild proves WaitChildTraces blocks until an
// in-flight (async) subagent's trace has flushed its records, so a one-shot run
// does not lose the child epoch at process exit. The child publishes its epoch
// and a usage turn, then EventDone after a delay; WaitChildTraces must not
// return until those records are on the shared writer.
func TestWaitChildTracesFlushesAsyncChild(t *testing.T) {
	parent := New(nil, tools.New(), permissions.New(permissions.ModeYolo, "", nil, nil, nil), "m")
	parent.System = "sys"
	var buf bytes.Buffer
	rootHandle := parent.AttachTraceSink(&buf)
	defer rootHandle.Close()

	// Give the parent a real epoch so the child has a root parent to point at.
	pe := parent.epochMgr.InitEpoch("session_start", EpochComponents{Model: "m"})

	child := New(nil, tools.New(), permissions.New(permissions.ModeYolo, "", nil, nil, nil), "m")
	if h := parent.AttachChildTraceSink(child); h == nil {
		t.Fatal("expected a child trace handle when a root trace is attached")
	}

	go func() {
		child.bus.Publish(EventEpochCreated{EpochID: "epoch_child", StaticPrefixHash: "CH", ToolsHash: "CT", Reason: "session_start"})
		child.bus.Publish(EventStepFinish{Reason: StopModelDone, Usage: llm.Usage{PromptCacheHitTokens: 10}})
		time.Sleep(40 * time.Millisecond)
		child.bus.Publish(EventDone{Reason: StopModelDone})
	}()

	parent.WaitChildTraces(2 * time.Second)

	if !strings.Contains(buf.String(), "epoch_child") {
		t.Fatalf("child epoch not flushed before WaitChildTraces returned:\n%s", buf.String())
	}

	type rec struct {
		EpochID       string `json:"epoch_id"`
		AgentRole     string `json:"agent_role"`
		ParentEpochID string `json:"parent_epoch_id"`
	}
	var sawChild bool
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var rr rec
		if err := json.Unmarshal([]byte(line), &rr); err != nil {
			t.Fatalf("malformed trace line %q: %v", line, err)
		}
		if rr.EpochID == "epoch_child" {
			sawChild = true
			if rr.AgentRole != "subagent" {
				t.Errorf("child record agent_role = %q, want subagent", rr.AgentRole)
			}
			if rr.ParentEpochID != pe.EpochID {
				t.Errorf("child parent_epoch_id = %q, want parent epoch %q", rr.ParentEpochID, pe.EpochID)
			}
		}
	}
	if !sawChild {
		t.Fatal("no child epoch record found in trace")
	}
}
