package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// planTestAgent builds a minimal Agent wired with real tools and a
// drain goroutine for events. Callers must not use Client (nil).
func planTestAgent(t *testing.T) (*Agent, func()) {
	t.Helper()
	reg := tools.New()
	// Register a read-only tool and a write tool so subset logic is exercised.
	reg.Register(readOnlyTool("grep"))
	reg.Register(writeTool("write_file"))
	reg.Register(readOnlyTool("plan_exit"))
	reg.Register(readOnlyTool("question"))

	pol := permissions.New(permissions.ModeDefault, "/repo", nil, nil, nil)
	a := New(nil, reg, pol, "test-model")
	// Drain events so emit doesn't block.
	done := make(chan struct{})
	go func() {
		for range a.Events() {
		}
		close(done)
	}()
	return a, func() {
		close(a.events)
		<-done
	}
}

type readOnlyTool string

func (t readOnlyTool) Name() string              { return string(t) }
func (readOnlyTool) Description() string         { return "ro" }
func (readOnlyTool) Parameters() json.RawMessage { return json.RawMessage(`{}`) }
func (readOnlyTool) Execute(_ context.Context, _ json.RawMessage) (tools.Result, error) {
	return tools.Result{}, nil
}
func (readOnlyTool) IsReadOnly() bool { return true }

type writeTool string

func (t writeTool) Name() string              { return string(t) }
func (writeTool) Description() string         { return "w" }
func (writeTool) Parameters() json.RawMessage { return json.RawMessage(`{}`) }
func (writeTool) Execute(_ context.Context, _ json.RawMessage) (tools.Result, error) {
	return tools.Result{}, nil
}
func (writeTool) IsReadOnly() bool { return false }

func TestEnterPlanSuccess(t *testing.T) {
	a, cleanup := planTestAgent(t)
	defer cleanup()

	if err := a.EnterPlan(context.Background()); err != nil {
		t.Fatalf("EnterPlan: unexpected error: %v", err)
	}
	if !a.inPlan {
		t.Error("inPlan should be true after EnterPlan")
	}
	// Only read-only tools + question + plan_exit should remain.
	for _, tool := range a.Tools.All() {
		ro, ok := tool.(tools.ReadOnlyHint)
		if !ok || !ro.IsReadOnly() {
			if tool.Name() != "question" && tool.Name() != "plan_exit" {
				t.Errorf("non-read-only tool %q should not be in plan registry", tool.Name())
			}
		}
	}
	if _, ok := a.Tools.Get("write_file"); ok {
		t.Error("write_file should be excluded from plan registry")
	}
	if a.Permissions.Mode != permissions.ModePlan {
		t.Errorf("permissions mode = %v, want ModePlan", a.Permissions.Mode)
	}
}

func TestEnterPlanAlreadyInPlan(t *testing.T) {
	a, cleanup := planTestAgent(t)
	defer cleanup()

	if err := a.EnterPlan(context.Background()); err != nil {
		t.Fatalf("first EnterPlan: %v", err)
	}
	if err := a.EnterPlan(context.Background()); err == nil {
		t.Fatal("expected error for second EnterPlan")
	}
}

func TestExitPlanSuccess(t *testing.T) {
	a, cleanup := planTestAgent(t)
	defer cleanup()

	origTools := a.Tools
	origPolicy := a.Permissions

	if err := a.EnterPlan(context.Background()); err != nil {
		t.Fatalf("EnterPlan: %v", err)
	}
	if err := a.ExitPlan(context.Background(), "step 1\nstep 2"); err != nil {
		t.Fatalf("ExitPlan: %v", err)
	}
	if a.inPlan {
		t.Error("inPlan should be false after ExitPlan")
	}
	if a.Tools != origTools {
		t.Error("Tools not restored to original")
	}
	if a.Permissions != origPolicy {
		t.Error("Permissions not restored to original")
	}
	if a.savedTools != nil {
		t.Error("savedTools should be nil after ExitPlan")
	}
	if a.savedPolicy != nil {
		t.Error("savedPolicy should be nil after ExitPlan")
	}
}

func TestExitPlanNotInPlan(t *testing.T) {
	a, cleanup := planTestAgent(t)
	defer cleanup()

	if err := a.ExitPlan(context.Background(), "plan"); err == nil {
		t.Fatal("expected error for ExitPlan when not in plan mode")
	}
}

func TestEnterThenExitThenEnter(t *testing.T) {
	a, cleanup := planTestAgent(t)
	defer cleanup()

	if err := a.EnterPlan(context.Background()); err != nil {
		t.Fatalf("EnterPlan(1): %v", err)
	}
	if err := a.ExitPlan(context.Background(), "x"); err != nil {
		t.Fatalf("ExitPlan(1): %v", err)
	}
	// Should be able to re-enter.
	if err := a.EnterPlan(context.Background()); err != nil {
		t.Fatalf("EnterPlan(2): %v", err)
	}
	if !a.inPlan {
		t.Error("inPlan should be true after re-enter")
	}
}

func TestPlanControllerCompileCheck(t *testing.T) {
	// The var _ tools.PlanController = (*Agent)(nil) line in plan.go
	// is a compile-time check; this test just documents it.
	var _ tools.PlanController = (*Agent)(nil)
}

func TestPlanErrorPropagation(t *testing.T) {
	a, cleanup := planTestAgent(t)
	defer cleanup()

	// EnterPlan error should propagate.
	if err := a.EnterPlan(context.Background()); err != nil {
		t.Fatalf("EnterPlan: %v", err)
	}
	err := a.EnterPlan(context.Background())
	if err == nil || err.Error() != "already in plan mode" {
		t.Errorf("got %v, want 'already in plan mode'", err)
	}

	// Exit then test ExitPlan error.
	if err := a.ExitPlan(context.Background(), "x"); err != nil {
		t.Fatalf("ExitPlan: %v", err)
	}
	err = a.ExitPlan(context.Background(), "x")
	if err == nil || err.Error() != "not in plan mode" {
		t.Errorf("got %v, want 'not in plan mode'", err)
	}
}

func TestPlanSaveRestoreIdentity(t *testing.T) {
	a, cleanup := planTestAgent(t)
	defer cleanup()

	origToolNames := toolNames(a.Tools)
	origMode := a.Permissions.Mode

	a.EnterPlan(context.Background())
	a.ExitPlan(context.Background(), "x")

	restoredNames := toolNames(a.Tools)
	if len(origToolNames) != len(restoredNames) {
		t.Errorf("tool count changed: %d -> %d", len(origToolNames), len(restoredNames))
	}
	for i, n := range origToolNames {
		if restoredNames[i] != n {
			t.Errorf("tool[%d] = %q, want %q", i, restoredNames[i], n)
		}
	}
	if a.Permissions.Mode != origMode {
		t.Errorf("mode = %v, want %v", a.Permissions.Mode, origMode)
	}
}

func toolNames(reg *tools.Registry) []string {
	var names []string
	for _, t := range reg.All() {
		names = append(names, t.Name())
	}
	return names
}
