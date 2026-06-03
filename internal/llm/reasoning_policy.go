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

// applyPolicy is a pure function — it copies messages and applies the policy
// without mutating the input slice or any Blocks slice within it.
func applyPolicy(messages []Message, p ReasoningPolicy) []Message {
	if p.Mode == PolicyPassThrough {
		return messages
	}

	// Collect indices of assistant messages (in order) so we can compute
	// the keep-window for PolicyRetainLast.
	assistantIndices := []int{}
	for i, m := range messages {
		if m.Role == "assistant" {
			assistantIndices = append(assistantIndices, i)
		}
	}

	keepFrom := len(assistantIndices) // exclusive lower bound: keep indices >= keepFrom
	if p.Mode == PolicyRetainLast {
		keepFrom = len(assistantIndices) - p.N
		if keepFrom < 0 {
			keepFrom = 0
		}
	}
	// For DropAll, keepFrom stays at len(assistantIndices), so no turn is kept.

	// Build a set of assistant-message indices that keep their reasoning.
	keepSet := map[int]bool{}
	for rank, idx := range assistantIndices {
		if rank >= keepFrom {
			keepSet[idx] = true
		}
	}

	out := make([]Message, len(messages))
	for i, m := range messages {
		if m.Role != "assistant" {
			out[i] = m
			continue
		}
		if keepSet[i] {
			out[i] = m
			continue
		}
		// Strip or replace ThinkingBlocks.
		newBlocks := make([]ContentBlock, 0, len(m.Blocks))
		for _, b := range m.Blocks {
			tb, isThinking := b.(ThinkingBlock)
			if !isThinking {
				newBlocks = append(newBlocks, b)
				continue
			}
			if p.Mode == PolicyRetainLast {
				// Replace with placeholder so DeepSeek still sees a thinking block
				// but we don't pay for the full reasoning tokens in the body.
				newBlocks = append(newBlocks, ThinkingBlock{Text: OmittedReasoningPlaceholder, Signature: tb.Signature})
			}
			// PolicyDropAll: omit the block entirely.
		}
		out[i] = Message{Role: m.Role, Blocks: newBlocks}
	}
	return out
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
