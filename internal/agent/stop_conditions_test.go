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
