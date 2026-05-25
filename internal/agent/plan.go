package agent

import (
	"context"
	"errors"

	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// Compile-time check: Agent implements tools.PlanController.
var _ tools.PlanController = (*Agent)(nil)

// EnterPlan transitions the agent into plan mode. While in plan mode
// only read-only tools, question, and plan_exit are available.
// Calling EnterPlan when already in plan mode returns an error.
func (a *Agent) EnterPlan(_ context.Context) error {
	if a.inPlan {
		return errors.New("already in plan mode")
	}

	// Save current state for restoration in ExitPlan.
	a.savedTools = a.Tools
	a.savedPolicy = a.Permissions

	// Build the whitelist: read-only tools + question + plan_exit.
	names := readOnlyToolNames(a.Tools)

	a.Tools = a.savedTools.Subset(names)
	a.Permissions = permissions.New(
		permissions.ModePlan,
		a.savedPolicy.Cwd,
		a.savedPolicy.SecretPathPatterns,
		a.savedPolicy.BashAllowlist(),
		a.savedPolicy.Rules,
	)
	a.inPlan = true

	a.bus.Publish(EventInfo{Text: "entered plan mode — only read-only tools are available until plan_exit"})
	return nil
}

// ExitPlan transitions the agent out of plan mode, restoring the
// original tool registry and permissions policy. plan is the finalized
// plan text (unused here; consumed by the plan_exit tool itself).
// Calling ExitPlan when not in plan mode returns an error.
func (a *Agent) ExitPlan(_ context.Context, plan string) error {
	if !a.inPlan {
		return errors.New("not in plan mode")
	}

	a.Tools = a.savedTools
	a.Permissions = a.savedPolicy
	a.inPlan = false
	a.savedTools = nil
	a.savedPolicy = nil

	a.bus.Publish(EventInfo{Text: "exited plan mode — full tool access restored"})
	return nil
}

// readOnlyToolNames returns the names of all read-only tools in reg,
// plus "question" and "plan_exit" which must always be available in
// plan mode.
func readOnlyToolNames(reg *tools.Registry) []string {
	var names []string
	for _, t := range reg.All() {
		if ro, ok := t.(tools.ReadOnlyHint); ok && ro.IsReadOnly() {
			names = append(names, t.Name())
		}
	}
	names = appendIfMissing(names, "question")
	names = appendIfMissing(names, "plan_exit")
	return names
}

func appendIfMissing(names []string, name string) []string {
	for _, n := range names {
		if n == name {
			return names
		}
	}
	return append(names, name)
}
