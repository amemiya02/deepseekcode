package agent

import (
	"context"
	"encoding/json"
)

// DuetDecision is what the Pro Validator returns about a proposed tool
// call. The agent renders this inline (◆ pro check) and honors blocks
// unless the user overrides via the TUI.
type DuetDecision struct {
	Approve   bool   `json:"approve"`
	Reasoning string `json:"reasoning"`
}

// DuetValidator decides whether a destructive tool call should proceed.
// Wave 5 will implement the real version that calls the pro model;
// the v0.1 scaffold ships with a no-op validator that always approves
// so the rest of the agent loop is testable end-to-end before pro
// integration lands.
type DuetValidator interface {
	Validate(ctx context.Context, toolName string, args json.RawMessage, transcript []byte) (DuetDecision, error)
}

// NopValidator is the development-time validator. Always approves.
type NopValidator struct{}

// Validate implements DuetValidator. Always approves with an empty
// reasoning string; the agent renders this as "◆ pro check: approved
// (validator not yet implemented)" to make the placeholder visible.
func (NopValidator) Validate(_ context.Context, _ string, _ json.RawMessage, _ []byte) (DuetDecision, error) {
	return DuetDecision{Approve: true, Reasoning: "validator not yet implemented"}, nil
}
