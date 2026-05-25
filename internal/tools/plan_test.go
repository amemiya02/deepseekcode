package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type fakePlanController struct {
	entered  bool
	exited   bool
	exitPlan string
	enterErr error
	exitErr  error
}

func (f *fakePlanController) EnterPlan(_ context.Context) error {
	f.entered = true
	return f.enterErr
}

func (f *fakePlanController) ExitPlan(_ context.Context, plan string) error {
	f.exited = true
	f.exitPlan = plan
	return f.exitErr
}

func TestPlanEnterTool(t *testing.T) {
	t.Run("basics", func(t *testing.T) {
		tool := NewPlanEnterTool(&fakePlanController{})
		if tool.Name() != "plan_enter" {
			t.Errorf("Name() = %q, want %q", tool.Name(), "plan_enter")
		}
		if !tool.IsReadOnly() {
			t.Error("IsReadOnly() should be true")
		}
	})

	t.Run("success", func(t *testing.T) {
		ctrl := &fakePlanController{}
		tool := NewPlanEnterTool(ctrl)
		res, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error result: %s", res.Content)
		}
		if !ctrl.entered {
			t.Error("EnterPlan not called")
		}
	})

	t.Run("nil controller", func(t *testing.T) {
		tool := NewPlanEnterTool(nil)
		res, _ := tool.Execute(context.Background(), json.RawMessage(`{}`))
		if !res.IsError {
			t.Fatal("expected error for nil controller")
		}
	})

	t.Run("controller error", func(t *testing.T) {
		ctrl := &fakePlanController{enterErr: errors.New("already in plan")}
		tool := NewPlanEnterTool(ctrl)
		res, _ := tool.Execute(context.Background(), json.RawMessage(`{}`))
		if !res.IsError {
			t.Fatal("expected error result")
		}
	})
}

func TestPlanExitTool(t *testing.T) {
	t.Run("basics", func(t *testing.T) {
		tool := NewPlanExitTool(&fakePlanController{})
		if tool.Name() != "plan_exit" {
			t.Errorf("Name() = %q, want %q", tool.Name(), "plan_exit")
		}
		if !tool.IsReadOnly() {
			t.Error("IsReadOnly() should be true")
		}
	})

	t.Run("success", func(t *testing.T) {
		ctrl := &fakePlanController{}
		tool := NewPlanExitTool(ctrl)
		res, err := tool.Execute(context.Background(), json.RawMessage(`{"plan":"step 1\nstep 2"}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error result: %s", res.Content)
		}
		if !ctrl.exited {
			t.Error("ExitPlan not called")
		}
		if ctrl.exitPlan != "step 1\nstep 2" {
			t.Errorf("ExitPlan plan = %q", ctrl.exitPlan)
		}
	})

	t.Run("missing plan", func(t *testing.T) {
		tool := NewPlanExitTool(&fakePlanController{})
		res, _ := tool.Execute(context.Background(), json.RawMessage(`{}`))
		if !res.IsError {
			t.Fatal("expected error for missing plan")
		}
	})

	t.Run("empty plan", func(t *testing.T) {
		tool := NewPlanExitTool(&fakePlanController{})
		res, _ := tool.Execute(context.Background(), json.RawMessage(`{"plan":""}`))
		if !res.IsError {
			t.Fatal("expected error for empty plan")
		}
	})

	t.Run("nil controller", func(t *testing.T) {
		tool := NewPlanExitTool(nil)
		res, _ := tool.Execute(context.Background(), json.RawMessage(`{"plan":"x"}`))
		if !res.IsError {
			t.Fatal("expected error for nil controller")
		}
	})

	t.Run("controller error", func(t *testing.T) {
		ctrl := &fakePlanController{exitErr: errors.New("not in plan")}
		tool := NewPlanExitTool(ctrl)
		res, _ := tool.Execute(context.Background(), json.RawMessage(`{"plan":"x"}`))
		if !res.IsError {
			t.Fatal("expected error result")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		tool := NewPlanExitTool(&fakePlanController{})
		res, _ := tool.Execute(context.Background(), json.RawMessage(`{bad`))
		if !res.IsError {
			t.Fatal("expected error for invalid json")
		}
	})
}
