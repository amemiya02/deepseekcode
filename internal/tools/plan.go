package tools

import (
	"context"
	"encoding/json"
)

// PlanController is implemented by the agent layer. It manages the
// agent's plan-mode state machine (enter/exit).
type PlanController interface {
	EnterPlan(ctx context.Context) error
	ExitPlan(ctx context.Context, plan string) error
}

// PlanEnterTool transitions the agent into plan mode.
type PlanEnterTool struct{ c PlanController }

// NewPlanEnterTool creates a PlanEnterTool backed by the given controller.
func NewPlanEnterTool(c PlanController) *PlanEnterTool { return &PlanEnterTool{c: c} }

func (*PlanEnterTool) Name() string { return "plan_enter" }

func (*PlanEnterTool) Description() string {
	return "Enter plan mode: the agent may only read and ask questions until plan_exit. " +
		"Use before proposing a multi-step change."
}

func (*PlanEnterTool) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	})
}

func (t *PlanEnterTool) Execute(ctx context.Context, _ json.RawMessage) (Result, error) {
	if t.c == nil {
		return Errf("plan_enter: no controller configured"), nil
	}
	if err := t.c.EnterPlan(ctx); err != nil {
		return Errf("plan_enter: %v", err), nil
	}
	return Result{Content: "Entered plan mode. Only read-only tools are available until plan_exit."}, nil
}

func (*PlanEnterTool) IsReadOnly() bool { return true }

// PlanExitTool transitions the agent out of plan mode.
type PlanExitTool struct{ c PlanController }

// NewPlanExitTool creates a PlanExitTool backed by the given controller.
func NewPlanExitTool(c PlanController) *PlanExitTool { return &PlanExitTool{c: c} }

func (*PlanExitTool) Name() string { return "plan_exit" }

func (*PlanExitTool) Description() string {
	return "Exit plan mode and present the finalized plan. Restores full tool access."
}

func (*PlanExitTool) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"plan": map[string]any{"type": "string"},
		},
		"required": []string{"plan"},
	})
}

func (t *PlanExitTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	if t.c == nil {
		return Errf("plan_exit: no controller configured"), nil
	}
	var p struct {
		Plan string `json:"plan"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("plan_exit: invalid args: %v", err), nil
	}
	if p.Plan == "" {
		return Errf("plan_exit: plan text is required"), nil
	}
	if err := t.c.ExitPlan(ctx, p.Plan); err != nil {
		return Errf("plan_exit: %v", err), nil
	}
	return Result{Content: "Plan finalized:\n" + p.Plan}, nil
}

func (*PlanExitTool) IsReadOnly() bool { return true }
