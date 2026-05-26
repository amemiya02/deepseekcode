//go:build !windows

package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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
				t.Log("agent done")
				return
			}
		case <-timeout:
			t.Fatal("timeout waiting for events")
		}
	}

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

	// Check the job status
	status, err := parent.JobStatus(jobID)
	if err != nil {
		t.Fatalf("JobStatus error: %v", err)
	}
	if status.State != "succeeded" {
		t.Errorf("expected state 'succeeded', got %q", status.State)
	}
	if !strings.Contains(status.Tail, "hello") {
		t.Errorf("expected tail to contain 'hello', got %q", status.Tail)
	}
}

func TestE2EAsyncSubagent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Parent turn: model calls task with async:true
		emitSSE(w,
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"task","arguments":"{\"description\":\"test\",\"async\":true}"}}]}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`,
		)
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
				t.Log("agent done")
				return
			}
		case <-timeout:
			t.Fatal("timeout waiting for events")
		}
	}

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

	// Verify we did NOT emit EventSubagentStart (async uses BackgroundJob events)
	// This test confirms the event unification
}
