package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/agents"
	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

func TestE2ESubagentEvents(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		switch n {
		case 1:
			// Parent turn: model calls "task" tool.
			emitSSE(w,
				`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"task","arguments":"{\"description\":\"find bugs\"}"}}]}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`,
			)
		case 2:
			// Child turn: final assistant text + stop.
			emitSSE(w,
				`{"choices":[{"index":0,"delta":{"content":"found 2 bugs"}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`,
			)
		case 3:
			// Parent turn after task result: model stops.
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

	// Set up LoopSpawner and register task tool.
	spawner := &LoopSpawner{
		Client: client,
		Parent: parent,
		Defs:   map[string]agents.AgentDef{},
	}
	reg.Register(tools.NewSubagentTool(spawner))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_, _ = parent.Run(ctx, "use the task tool to find bugs")
	}()

	var sawStart, sawFinish bool
	var startAgent, finishAgent string

	for ev := range parent.Events() {
		switch e := ev.(type) {
		case EventSubagentStart:
			sawStart = true
			startAgent = e.Agent
		case EventSubagentFinish:
			sawFinish = true
			finishAgent = e.Agent
		case EventDone:
			cancel()
			return
		}
	}

	if !sawStart {
		t.Error("never received EventSubagentStart")
	}
	if !sawFinish {
		t.Error("never received EventSubagentFinish")
	}
	if startAgent != "" {
		t.Errorf("EventSubagentStart.Agent = %q, want empty (generic)", startAgent)
	}
	if finishAgent != "" {
		t.Errorf("EventSubagentFinish.Agent = %q, want empty (generic)", finishAgent)
	}
}

func TestE2ESubagentFailureEvent(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		switch n {
		case 1:
			// Parent turn: model calls "task" tool.
			emitSSE(w,
				`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"task","arguments":"{\"description\":\"fail\"}"}}]}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`,
			)
		case 2:
			// Child turn: server error → child fails.
			http.Error(w, "server error", http.StatusInternalServerError)
		case 3:
			// Parent turn after failed task result: model stops.
			emitSSE(w,
				`{"choices":[{"index":0,"delta":{"content":"ok"}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":15,"completion_tokens":1,"total_tokens":16}}`,
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

	spawner := &LoopSpawner{
		Client: client,
		Parent: parent,
		Defs:   map[string]agents.AgentDef{},
	}
	reg.Register(tools.NewSubagentTool(spawner))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_, _ = parent.Run(ctx, "use task to fail")
	}()

	var sawStart, sawFinish bool

	for ev := range parent.Events() {
		switch ev.(type) {
		case EventSubagentStart:
			sawStart = true
		case EventSubagentFinish:
			sawFinish = true
		case EventDone:
			cancel()
			return
		}
	}

	if !sawStart {
		t.Error("never received EventSubagentStart on failure")
	}
	if !sawFinish {
		t.Error("never received EventSubagentFinish on failure")
	}
}
