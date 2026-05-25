package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/agents"
	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// writeStubTool is a non-read-only stub tool for testing.
type writeStubTool struct{ name string }

func (w writeStubTool) Name() string              { return w.name }
func (writeStubTool) Description() string         { return "write" }
func (writeStubTool) Parameters() json.RawMessage { return json.RawMessage(`{}`) }
func (writeStubTool) Execute(_ context.Context, _ json.RawMessage) (tools.Result, error) {
	return tools.Result{}, nil
}
func (writeStubTool) IsReadOnly() bool { return false }

func TestSpawnPlanSubagentDirectly(t *testing.T) {
	// Test Spawn with mode:plan by calling it directly (no parent Run loop).
	var childCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&childCalls, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		emitSSE(w,
			`{"choices":[{"index":0,"delta":{"content":"auth design: use JWT"}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`,
		)
	}))
	defer srv.Close()

	client := llm.NewClient("k", srv.URL)

	parentReg := tools.New()
	parentReg.Register(stubTool{name: "read_file"})
	parentReg.Register(writeStubTool{name: "write_file"})
	parentReg.Register(stubTool{name: "grep"})

	pol := permissions.New(permissions.ModeYolo, "/repo", nil, nil, nil)
	parent := New(client, parentReg, pol, "test-model")
	parent.System = "sys"
	parent.StopWhen = []StopCondition{MaxSteps(10)}

	defs := map[string]agents.AgentDef{
		"architect": {
			Name:  "architect",
			Mode:  "plan",
			Tools: []string{"read_file", "write_file", "grep"},
		},
	}

	spawner := &LoopSpawner{
		Client: client,
		Parent: parent,
		Defs:   defs,
	}

	// Drain parent events so Spawn's emits don't block.
	done := make(chan struct{})
	go func() {
		for range parent.Events() {
		}
		close(done)
	}()

	result, err := spawner.Spawn(context.Background(), tools.SpawnRequest{
		Agent:       "architect",
		Description: "design auth",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	if result.StepCount < 1 {
		t.Errorf("StepCount = %d, want >= 1", result.StepCount)
	}
	if result.Summary == "" {
		t.Error("Summary should not be empty")
	}

	// Verify the child made an HTTP request.
	if atomic.LoadInt32(&childCalls) == 0 {
		t.Error("child server received no requests")
	}

	// Close parent events to drain goroutine.
	close(parent.events)
	<-done
}

func TestPlanSubagentRegistryFiltered(t *testing.T) {
	parentReg := tools.New()
	parentReg.Register(stubTool{name: "read_file"})
	parentReg.Register(writeStubTool{name: "write_file"})
	parentReg.Register(writeStubTool{name: "bash"})
	parentReg.Register(stubTool{name: "grep"})

	parent := New(nil, parentReg, permissions.New(permissions.ModeDefault, "", nil, nil, nil), "m")
	parent.System = "s"

	def := agents.AgentDef{
		Name:  "planner",
		Mode:  "plan",
		Tools: []string{"read_file", "write_file", "bash"},
	}
	names := def.Tools
	names = remove(names, "task")

	sub := parent.Tools.Subset(names)
	planNames := readOnlyToolNames(sub)
	childReg := parent.Tools.Subset(planNames)

	for _, bad := range []string{"write_file", "bash", "task"} {
		if _, ok := childReg.Get(bad); ok {
			t.Errorf("plan registry should not contain %q", bad)
		}
	}
	for _, good := range []string{"read_file"} {
		if _, ok := childReg.Get(good); !ok {
			t.Errorf("plan registry should contain %q", good)
		}
	}
	// "grep" is not in def.Tools, so it's correctly absent from the plan registry.
}

func TestPlanSubagentPolicyMode(t *testing.T) {
	parentPol := permissions.New(permissions.ModeDefault, "/repo", nil, nil, nil)
	childPol := parentPol.DeriveChild(permissions.ModePlan)
	if childPol.Mode != permissions.ModePlan {
		t.Errorf("child mode = %v, want ModePlan", childPol.Mode)
	}

	roPol := permissions.New(permissions.ModeReadOnly, "/repo", nil, nil, nil)
	childPol2 := roPol.DeriveChild(permissions.ModePlan)
	if childPol2.Mode != permissions.ModeReadOnly {
		t.Errorf("ReadOnly parent + ModePlan child = %v, want ReadOnly (clamped)", childPol2.Mode)
	}
}
