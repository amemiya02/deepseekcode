//go:build !windows

package agent

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestE2EBackgroundBash(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Parent turn: model calls background_bash
		emitSSE(w,
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"background_bash","arguments":"{\"command\":\"echo hello\"}"}}]}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`,
		)
	}))
	defer srv.Close()

	client := llm.NewClient("k", srv.URL)
	reg := tools.New()
	pol := permissions.New(permissions.ModeYolo, "", nil, nil, nil)
	parent := New(client, reg, pol, "test-model")
	parent.System = "sys"
	parent.StopWhen = []StopCondition{MaxSteps(2)}

	reg.Register(tools.NewBackgroundBashTool(parent))
	reg.Register(tools.NewTaskStatusTool(parent))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_, _ = parent.Run(ctx, "run a background echo command")
	}()

	var sawStart, sawFinish bool
	var jobID string
	var jobKind string

	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev := <-parent.Events():
			switch e := ev.(type) {
			case EventBackgroundJobStart:
				sawStart = true
				jobID = e.ID
				jobKind = e.Kind.String()
			case EventBackgroundJobFinish:
				sawFinish = true
			case EventDone:
				// Continue waiting for background job finish
			}
			// Exit loop when we have both start and finish
			if sawStart && sawFinish {
				goto done
			}
		case <-timeout:
			t.Fatal("timeout waiting for events")
		}
	}
done:

	if !sawStart {
		t.Error("expected EventBackgroundJobStart")
	}
	if !sawFinish {
		t.Error("expected EventBackgroundJobFinish")
	}
	if jobID == "" {
		t.Error("expected job ID")
	}
	if jobKind != "background_bash" {
		t.Errorf("expected kind 'background_bash', got %q", jobKind)
	}

	// Check the job status via TaskStatusTool.Execute (T-2805 acceptance).
	taskStatusTool := tools.NewTaskStatusTool(parent)
	res, err := taskStatusTool.Execute(context.Background(),
		json.RawMessage(fmt.Sprintf(`{"job_id":%q,"tail_lines":200}`, jobID)))
	if err != nil {
		t.Fatalf("TaskStatusTool.Execute error: %v", err)
	}
	if res.IsError {
		t.Fatalf("task_status returned error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "state: succeeded") {
		t.Errorf("expected 'state: succeeded' in content, got %q", res.Content)
	}
	if !strings.Contains(res.Content, "hello") {
		t.Errorf("expected 'hello' in content, got %q", res.Content)
	}
}

func TestE2EAsyncSubagent(t *testing.T) {
	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&reqCount, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if n == 1 {
			// Parent turn: model calls task with async:true
			emitSSE(w,
				`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"task","arguments":"{\"description\":\"test\",\"async\":true}"}}]}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`,
			)
		} else if n == 2 {
			// Child subagent: returns final text
			emitSSE(w,
				`{"choices":[{"index":0,"delta":{"content":"subagent result: found 3 items"}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`,
			)
		} else {
			emitSSE(w,
				`{"choices":[{"index":0,"delta":{"content":"subagent result: found 3 items"}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`,
			)
		}
	}))
	defer srv.Close()

	client := llm.NewClient("k", srv.URL)
	reg := tools.New()
	reg.Register(stubTool{name: "read_file"})
	pol := permissions.New(permissions.ModeYolo, "", nil, nil, nil)
	parent := New(client, reg, pol, "test-model")
	parent.System = "sys"
	parent.StopWhen = []StopCondition{MaxSteps(2)}

	// Set up spawner
	spawner := &LoopSpawner{
		Client: client,
		Parent: parent,
		Defs:   map[string]agents.AgentDef{},
	}
	reg.Register(tools.NewSubagentTool(spawner))
	reg.Register(tools.NewTaskStatusTool(parent))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_, _ = parent.Run(ctx, "run an async task")
	}()

	var sawStart, sawFinish bool
	var sawSubagentStart bool
	var jobID string
	var jobKind string
	var finishState JobState

	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev := <-parent.Events():
			switch e := ev.(type) {
			case EventSubagentStart:
				sawSubagentStart = true
			case EventBackgroundJobStart:
				sawStart = true
				jobID = e.ID
				jobKind = e.Kind.String()
			case EventBackgroundJobFinish:
				sawFinish = true
				finishState = e.State
			case EventDone:
				// Continue waiting for background job finish
			}
			// Exit loop when we have both start and finish
			if sawStart && sawFinish {
				goto done
			}
		case <-timeout:
			t.Fatal("timeout waiting for events")
		}
	}
done:

	if !sawStart {
		t.Error("expected EventBackgroundJobStart")
	}
	if !sawFinish {
		t.Error("expected EventBackgroundJobFinish")
	}
	if jobID == "" {
		t.Error("expected job ID")
	}
	if jobKind != "subagent" {
		t.Errorf("expected kind 'subagent', got %q", jobKind)
	}

	// T-2805: async subagent MUST NOT emit EventSubagentStart
	if sawSubagentStart {
		t.Error("async subagent must NOT emit EventSubagentStart (uses BackgroundJob events)")
	}

	// T-2805: verify via TaskStatusTool.Execute
	taskStatusTool := tools.NewTaskStatusTool(parent)
	res, err := taskStatusTool.Execute(context.Background(),
		json.RawMessage(fmt.Sprintf(`{"job_id":%q,"tail_lines":200}`, jobID)))
	if err != nil {
		t.Fatalf("TaskStatusTool.Execute error: %v", err)
	}
	if res.IsError {
		t.Fatalf("task_status returned error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "state: succeeded") {
		t.Errorf("expected 'state: succeeded' in content, got %q", res.Content)
	}
	if !strings.Contains(res.Content, "subagent result") {
		t.Errorf("expected 'subagent result' in content, got %q", res.Content)
	}

	if finishState != JobSucceeded {
		t.Errorf("expected finish state JobSucceeded, got %v", finishState)
	}
}
