package agent

import (
	"github.com/amemiya02/deepseekcode/internal/llm"
)

// FoldPlan describes a proposed range of messages to summarize.
// [FromIdx, ToIdx) is the half-open interval of messages to fold.
type FoldPlan struct {
	FromIdx int
	ToIdx   int
	Reason  string
}

// PlanContextFold chooses which messages to summarize while preserving
// an immutable prefix and a recent tail. It returns (plan, true) when
// folding is needed, or (zero, false) when the message list fits.
//
// Parameters:
//   - messages: the current conversation
//   - maxTokens: total token budget; folding triggers when estimated
//     tokens exceed this threshold
//   - tailTokens: minimum token count to preserve in the recent tail
//   - estimate: token estimator function for a single message
//
// The function is pure: it does not mutate messages or any shared state.
func PlanContextFold(messages []llm.Message, maxTokens, tailTokens int, estimate func(llm.Message) int) (FoldPlan, bool) {
	if len(messages) == 0 {
		return FoldPlan{}, false
	}

	// Estimate total tokens across all messages.
	totalTokens := 0
	for _, m := range messages {
		totalTokens += estimate(m)
	}

	// Under budget: no folding needed.
	if totalTokens <= maxTokens {
		return FoldPlan{}, false
	}

	// Find the tail boundary: preserve at least tailTokens worth of
	// recent messages. Work backwards from the end.
	tailStart := len(messages)
	tailSum := 0
	for i := len(messages) - 1; i >= 0; i-- {
		tailSum += estimate(messages[i])
		if tailTokens <= 0 || tailSum >= tailTokens {
			tailStart = i
			break
		}
	}

	// If tailTokens > 0 and even the full message list cannot satisfy
	// the requested preserved tail token sum, there is no safe fold range.
	if tailTokens > 0 && tailSum < tailTokens {
		return FoldPlan{}, false
	}

	// Ensure at least one message is preserved in the tail.
	if tailStart >= len(messages) {
		tailStart = len(messages) - 1
	}

	// Never fold the final message if it's an assistant with tool calls.
	if tailStart == len(messages)-1 {
		last := messages[len(messages)-1]
		if last.Role == "assistant" && hasToolCalls(last) {
			// Move tail start back to exclude this message from folding.
			if tailStart > 0 {
				tailStart--
			} else {
				// Can't fold anything if the only message is a tool-call assistant.
				return FoldPlan{}, false
			}
		}
	}

	// The fold range is [0, tailStart). But we need to ensure we don't
	// include an assistant tool-call message at the boundary.
	fromIdx := 0
	toIdx := tailStart

	// Adjust toIdx backward if it would include an assistant tool-call
	// message at the boundary.
	for toIdx > 0 && toIdx <= len(messages) {
		prev := messages[toIdx-1]
		if prev.Role == "assistant" && hasToolCalls(prev) {
			toIdx--
		} else {
			break
		}
	}

	// If no safe fold range exists, return false.
	if toIdx <= fromIdx {
		return FoldPlan{}, false
	}

	return FoldPlan{
		FromIdx: fromIdx,
		ToIdx:   toIdx,
		Reason:  "context_fold: estimated tokens exceeded budget",
	}, true
}

// hasToolCalls checks if a message contains any tool_use blocks.
func hasToolCalls(m llm.Message) bool {
	for _, b := range m.Blocks {
		if _, ok := b.(llm.ToolUseBlock); ok {
			return true
		}
	}
	return false
}
