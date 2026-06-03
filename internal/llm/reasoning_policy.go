// internal/llm/reasoning_policy.go
package llm

import (
	"fmt"
	"os"
	"strconv"
)

type PolicyMode int

const (
	PolicyPassThrough PolicyMode = iota
	PolicyDropAll
	PolicyRetainLast
)

func (m PolicyMode) String() string {
	switch m {
	case PolicyPassThrough:
		return "PassThrough"
	case PolicyDropAll:
		return "DropAll"
	case PolicyRetainLast:
		return "RetainLast"
	default:
		return fmt.Sprintf("PolicyMode(%d)", int(m))
	}
}

type ReasoningPolicy struct {
	Mode PolicyMode
	N    int // only meaningful for PolicyRetainLast
}

// ReadPolicy reads DEEPSEEKCODE_REASONING_DROP and DEEPSEEKCODE_REASONING_RETAIN.
// Both unset → PassThrough. Only DROP=1 → DropAll. Only RETAIN=N → RetainLast(N).
// Both set → error. RETAIN=0 or negative → error.
func ReadPolicy() (ReasoningPolicy, error) {
	drop := os.Getenv("DEEPSEEKCODE_REASONING_DROP")
	retain := os.Getenv("DEEPSEEKCODE_REASONING_RETAIN")

	if drop != "" && retain != "" {
		return ReasoningPolicy{}, fmt.Errorf(
			"cannot set both DEEPSEEKCODE_REASONING_DROP and DEEPSEEKCODE_REASONING_RETAIN")
	}
	if drop != "" {
		if drop != "1" {
			return ReasoningPolicy{}, fmt.Errorf(
				"DEEPSEEKCODE_REASONING_DROP must be \"1\", got %q", drop)
		}
		return ReasoningPolicy{Mode: PolicyDropAll}, nil
	}
	if retain != "" {
		n, err := strconv.Atoi(retain)
		if err != nil || n <= 0 {
			return ReasoningPolicy{}, fmt.Errorf(
				"DEEPSEEKCODE_REASONING_RETAIN must be a positive integer, got %q", retain)
		}
		return ReasoningPolicy{Mode: PolicyRetainLast, N: n}, nil
	}
	return ReasoningPolicy{Mode: PolicyPassThrough}, nil
}
