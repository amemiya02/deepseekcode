package agent

import (
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

func TestMaxSteps(t *testing.T) {
	cond := MaxSteps(3)
	for i := 1; i <= 4; i++ {
		stop, reason := cond(make([]StepRecord, i))
		if i < 3 && stop {
			t.Errorf("i=%d: stopped early", i)
		}
		if i >= 3 && !stop {
			t.Errorf("i=%d: should have stopped", i)
		}
		if i >= 3 && reason != StopMaxSteps {
			t.Errorf("i=%d: reason=%v want %v", i, reason, StopMaxSteps)
		}
	}
}

func TestLoopDetection(t *testing.T) {
	cond := LoopDetection(5, 3)
	mkCall := func(name, args string) llm.ToolCall {
		return llm.ToolCall{Function: llm.ToolCallFunc{Name: name, Arguments: args}}
	}

	steps := []StepRecord{
		{ToolCalls: []llm.ToolCall{mkCall("read_file", `{"path":"a"}`)}},
		{ToolCalls: []llm.ToolCall{mkCall("read_file", `{"path":"a"}`)}},
		{ToolCalls: []llm.ToolCall{mkCall("read_file", `{"path":"a"}`)}},
	}
	stop, reason := cond(steps)
	if !stop {
		t.Errorf("expected stop on 3 identical calls")
	}
	if reason != StopLoopDetected {
		t.Errorf("reason=%v want %v", reason, StopLoopDetected)
	}

	varied := []StepRecord{
		{ToolCalls: []llm.ToolCall{mkCall("read_file", `{"path":"a"}`)}},
		{ToolCalls: []llm.ToolCall{mkCall("read_file", `{"path":"b"}`)}},
		{ToolCalls: []llm.ToolCall{mkCall("read_file", `{"path":"c"}`)}},
	}
	stop, _ = cond(varied)
	if stop {
		t.Errorf("varied args should not trigger loop detection")
	}
}

func TestStopReasonString(t *testing.T) {
	if s := StopVerifiedDone.String(); s != "verified_done" {
		t.Errorf("StopVerifiedDone.String() = %q, want %q", s, "verified_done")
	}
}

func TestMaxStepsDoesNotFireEarly(t *testing.T) {
	cond := MaxSteps(3)
	for n := 0; n < 3; n++ {
		steps := make([]StepRecord, n)
		stop, _ := cond(steps)
		if stop {
			t.Fatalf("MaxSteps(3) fired at len=%d, want to fire at len>=3", n)
		}
	}
	steps := make([]StepRecord, 3)
	stop, reason := cond(steps)
	if !stop {
		t.Fatal("MaxSteps(3) should fire at len=3")
	}
	if reason != StopMaxSteps {
		t.Fatalf("reason = %v, want StopMaxSteps", reason)
	}
}

func TestLoopDetectionDoesNotFireBeforeThreshold(t *testing.T) {
	cond := LoopDetection(5, 3)
	makeStep := func(name, args string) StepRecord {
		return StepRecord{ToolCalls: []llm.ToolCall{{Function: llm.ToolCallFunc{Name: name, Arguments: args}}}}
	}
	steps := []StepRecord{
		makeStep("edit", `{"file":"a"}`),
		makeStep("edit", `{"file":"a"}`),
	}
	stop, _ := cond(steps)
	if stop {
		t.Fatal("LoopDetection should not fire at 2 repeats (threshold=3)")
	}
	steps = append(steps, makeStep("edit", `{"file":"a"}`))
	stop, reason := cond(steps)
	if !stop {
		t.Fatal("LoopDetection should fire at 3 repeats")
	}
	if reason != StopLoopDetected {
		t.Fatalf("reason = %v, want StopLoopDetected", reason)
	}
}
