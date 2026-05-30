package tui

import (
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/agent"
	"github.com/amemiya02/deepseekcode/internal/llm"
)

// Rework Task-004 crit 2: /effort max changes Agent.ReasoningEffort
// and status.reasoningEffort; an info row confirms the change.
func TestSlashEffortSet(t *testing.T) {
	ag := &agent.Agent{ReasoningEffort: llm.ReasoningEffortHigh}
	sb := NewScrollback()
	a := &App{
		agent:      ag,
		scrollback: sb,
		status:     statusState{reasoningEffort: llm.ReasoningEffortHigh},
	}
	a.handleSlash("/effort max")
	if ag.ReasoningEffort != llm.ReasoningEffortMax {
		t.Errorf("agent effort = %s, want max", ag.ReasoningEffort)
	}
	if a.status.reasoningEffort != llm.ReasoningEffortMax {
		t.Errorf("status effort = %s, want max", a.status.reasoningEffort)
	}
	// Info row should contain "-> max".
	items := sb.Items()
	if len(items) == 0 {
		t.Fatal("expected info row, got 0 items")
	}
	last := items[len(items)-1]
	if last.kind != itemInfo {
		t.Errorf("last item kind = %d, want itemInfo", last.kind)
	}
	if !strings.Contains(last.text, "-> max") {
		t.Errorf("info row %q should contain '-> max'", last.text)
	}
}

// Rework Task-004 crit 3: /effort nope is rejected with an error listing
// allowed values; effort is unchanged.
func TestSlashEffortInvalid(t *testing.T) {
	ag := &agent.Agent{ReasoningEffort: llm.ReasoningEffortHigh}
	sb := NewScrollback()
	a := &App{
		agent:      ag,
		scrollback: sb,
		status:     statusState{reasoningEffort: llm.ReasoningEffortHigh},
	}
	a.handleSlash("/effort nope")
	// Effort must be unchanged.
	if ag.ReasoningEffort != llm.ReasoningEffortHigh {
		t.Errorf("agent effort = %s, want high (unchanged)", ag.ReasoningEffort)
	}
	if a.status.reasoningEffort != llm.ReasoningEffortHigh {
		t.Errorf("status effort = %s, want high (unchanged)", a.status.reasoningEffort)
	}
	// Error row should list allowed values.
	items := sb.Items()
	if len(items) == 0 {
		t.Fatal("expected error row, got 0 items")
	}
	last := items[len(items)-1]
	if last.kind != itemError {
		t.Errorf("last item kind = %d, want itemError", last.kind)
	}
	for _, want := range []string{"low", "medium", "high", "max"} {
		if !strings.Contains(last.text, want) {
			t.Errorf("error row %q should list %q", last.text, want)
		}
	}
}
