// internal/agent/route_turn_test.go
package agent

import (
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

func TestRouteTurnDisabledReturnsLoopModel(t *testing.T) {
	a := &Agent{Model: "deepseek-v4-flash"} // AutoRoute false
	model, _, _ := a.routeTurn("anything", 0)
	if model != "deepseek-v4-flash" {
		t.Fatalf("disabled routing must keep loop model, got %q", model)
	}
}

func TestRouteTurnEscalatesHardPrompt(t *testing.T) {
	a := &Agent{Model: "deepseek-v4-flash", EscalationModel: "deepseek-v4-pro", AutoRoute: true}
	model, thinking, effort := a.routeTurn("why does this deadlock? prove the root cause", 0)
	if model != "deepseek-v4-pro" || !thinking || effort != llm.ReasoningEffortMax {
		t.Fatalf("hard prompt should route pro+think+max, got %q %v %q", model, thinking, effort)
	}
}

func TestRouteTurnDefaultKeepsConfiguredEffort(t *testing.T) {
	a := &Agent{Model: "deepseek-v4-flash", ReasoningEffort: llm.ReasoningEffortHigh}
	model, thinking, effort := a.routeTurn("read a file", 0)
	if model != "deepseek-v4-flash" || !thinking || effort != llm.ReasoningEffortHigh {
		t.Fatalf("default route changed legacy behavior: %q %v %q", model, thinking, effort)
	}
}
