package agent

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// StepRecord captures one step's outcome so stop conditions can reason
// across history.
type StepRecord struct {
	FinishReason      string
	Usage             llm.Usage
	ToolCalls         []llm.ToolCall
	EpochID           string
	StaticPrefixHash  string
	ExpectedCacheMiss bool
}

// StopReason describes why the loop terminated.
type StopReason int

const (
	StopUnknown       StopReason = iota
	StopModelDone                // finish_reason!=tool_calls and no tool calls
	StopMaxSteps                 // step cap exceeded
	StopLoopDetected             // same tool call repeated too many times
	StopContextCancel            // ctx.Err()
	StopUserRequested            // explicit cancellation from TUI
)

func (r StopReason) String() string {
	switch r {
	case StopModelDone:
		return "model_done"
	case StopMaxSteps:
		return "max_steps"
	case StopLoopDetected:
		return "loop_detected"
	case StopContextCancel:
		return "context_cancel"
	case StopUserRequested:
		return "user_requested"
	}
	return "unknown"
}

// StopCondition examines recent history and returns (true, reason) when
// the loop should terminate. The agent calls all conditions after each
// step and stops on the first that fires.
type StopCondition func(steps []StepRecord) (stop bool, reason StopReason)

// MaxSteps caps total agent steps in a single Run. Default 50 in v0.1.
func MaxSteps(n int) StopCondition {
	return func(steps []StepRecord) (bool, StopReason) {
		if len(steps) >= n {
			return true, StopMaxSteps
		}
		return false, StopUnknown
	}
}

// LoopDetection breaks the loop when the same tool call (name + arg
// hash) appears `maxRepeats` times within the last `window` steps.
// Crush calls this in internal/agent/loop_detection.go; we use the same
// shape. Default v0.1: window=5, maxRepeats=3.
func LoopDetection(window, maxRepeats int) StopCondition {
	return func(steps []StepRecord) (bool, StopReason) {
		if len(steps) < maxRepeats {
			return false, StopUnknown
		}
		start := len(steps) - window
		if start < 0 {
			start = 0
		}
		counts := map[string]int{}
		for _, s := range steps[start:] {
			for _, tc := range s.ToolCalls {
				key := tc.Function.Name + ":" + sha8(tc.Function.Arguments)
				counts[key]++
				if counts[key] >= maxRepeats {
					return true, StopLoopDetected
				}
			}
		}
		return false, StopUnknown
	}
}

func sha8(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:4])
}
