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
	a := &Agent{Model: "deepseek-v4-flash", Thinking: true, ReasoningEffort: llm.ReasoningEffortHigh}
	model, thinking, effort := a.routeTurn("read a file", 0)
	if model != "deepseek-v4-flash" || !thinking || effort != llm.ReasoningEffortHigh {
		t.Fatalf("default route changed legacy behavior: %q %v %q", model, thinking, effort)
	}
}

// TestThinkingModeOverrides locks the cost-A/B knob: off never thinks, adaptive
// thinks only on turn 0 or a repair turn, and empty mode preserves legacy behavior.
func TestThinkingModeOverrides(t *testing.T) {
	// off: never think + no effort on the wire, even with Thinking=true.
	off := &Agent{Model: "m", Thinking: true, ThinkingMode: "off"}
	if _, think, eff := off.routeTurn("debug this error", 0); think || eff != "" {
		t.Fatalf("off must disable thinking, got think=%v eff=%q", think, eff)
	}
	// A substantive message is eligible to think; "x"-style trivial stand-ins are
	// not (see the trivial-greeting cases below).
	const task = "implement the review panel"
	// adaptive: think on turn 0 (plan), terse after, think again on a repair turn.
	adapt := &Agent{Model: "m", Thinking: true, ThinkingMode: "adaptive"}
	if _, think, _ := adapt.routeTurn(task, 0); !think {
		t.Fatal("adaptive turn 0 must think (plan)")
	}
	adapt.turnsSeen = 1
	if _, think, _ := adapt.routeTurn(task, 0); think {
		t.Fatal("adaptive turn>0 must be terse")
	}
	if _, think, _ := adapt.routeTurn(task, 1); !think {
		t.Fatal("adaptive repair turn must think")
	}
	// empty mode: legacy Thinking honored.
	on := &Agent{Model: "m", Thinking: true}
	if _, think, _ := on.routeTurn(task, 0); !think {
		t.Fatal("empty mode must keep legacy Thinking=true")
	}

	// Trivial greetings/acks must NOT force thinking — neither on the adaptive
	// first turn nor in legacy default-on mode (the redundant-thinking-block fix).
	adapt2 := &Agent{Model: "m", Thinking: true, ThinkingMode: "adaptive"}
	if _, think, _ := adapt2.routeTurn("你好", 0); think {
		t.Fatal("adaptive turn 0 must NOT think for a trivial greeting")
	}
	if _, think, _ := on.routeTurn("hi", 0); think {
		t.Fatal("default-on must NOT think for a trivial greeting")
	}
	// ...but a short message carrying a high-effort keyword still thinks.
	if _, think, _ := on.routeTurn("报错", 0); !think {
		t.Fatal("a high-effort keyword must think even when short")
	}
}
