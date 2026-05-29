package session

import "github.com/amemiya02/deepseekcode/internal/llm"

// interruptedToolResultContent is the placeholder body for a tool result
// synthesized during replay when the original was never recorded — typically
// because the process was interrupted between persisting the assistant turn
// that issued the tool call and persisting the tool's result.
const interruptedToolResultContent = "(no tool result recorded — the session was interrupted before this tool call completed)"

// repairDanglingToolCalls makes a replayed history safe to send to DeepSeek:
// every assistant tool_call must be followed by a matching tool result, or
// the API rejects the request. A crash between persisting an assistant turn
// and its tool results leaves a dangling tool_call; this synthesizes a
// placeholder result (marked IsError) for any tool_call id that has no
// recorded result, inserted immediately after the assistant message that
// issued it.
//
// It is a no-op (returns the input slice unchanged, no allocation) when every
// tool_call is already paired, so the normal path — and the cache-stable wire
// bytes built from it — is untouched. Reasoning-block stamping is handled
// separately by llm.SanitizeForDeepSeek at request-build time, so this only
// concerns tool-call pairing.
func repairDanglingToolCalls(msgs []Message) []Message {
	if !hasDanglingToolCall(msgs) {
		return msgs
	}

	out := make([]Message, 0, len(msgs)+2)
	for i := 0; i < len(msgs); i++ {
		m := msgs[i]
		out = append(out, m)

		ids := toolUseIDs(m)
		if len(ids) == 0 {
			continue
		}

		// Collect the result ids already present in the contiguous run of
		// tool messages that follows this assistant message.
		present := make(map[string]bool)
		for j := i + 1; j < len(msgs) && msgs[j].Role == "tool"; j++ {
			for _, id := range toolResultIDs(msgs[j]) {
				present[id] = true
			}
		}

		// Synthesize a result for every unmatched id, in the assistant's own
		// tool_call order, placed right after the assistant message (ahead of
		// any real results, which the API matches by id regardless of order).
		for _, id := range ids {
			if id != "" && !present[id] {
				out = append(out, syntheticToolResult(id))
			}
		}
	}
	return out
}

// hasDanglingToolCall reports whether any assistant tool_call lacks a matching
// tool result in the tool-message run that follows it.
func hasDanglingToolCall(msgs []Message) bool {
	for i, m := range msgs {
		ids := toolUseIDs(m)
		if len(ids) == 0 {
			continue
		}
		present := make(map[string]bool)
		for j := i + 1; j < len(msgs) && msgs[j].Role == "tool"; j++ {
			for _, id := range toolResultIDs(msgs[j]) {
				present[id] = true
			}
		}
		for _, id := range ids {
			if id != "" && !present[id] {
				return true
			}
		}
	}
	return false
}

// toolUseIDs returns the ids of the tool_call blocks in an assistant message.
func toolUseIDs(m Message) []string {
	if m.Role != "assistant" {
		return nil
	}
	var ids []string
	for _, b := range m.Blocks {
		if tu, ok := b.(llm.ToolUseBlock); ok {
			ids = append(ids, tu.ID)
		}
	}
	return ids
}

// toolResultIDs returns the tool_use ids a tool message carries results for.
func toolResultIDs(m Message) []string {
	if m.Role != "tool" {
		return nil
	}
	var ids []string
	for _, b := range m.Blocks {
		if tr, ok := b.(llm.ToolResultBlock); ok {
			ids = append(ids, tr.ToolUseID)
		}
	}
	// Legacy rows may carry the id only in the column-derived field.
	if len(ids) == 0 && m.ToolCallID != "" {
		ids = append(ids, m.ToolCallID)
	}
	return ids
}

func syntheticToolResult(id string) Message {
	return Message{
		Role:       "tool",
		ToolCallID: id,
		Content:    interruptedToolResultContent,
		Blocks: []llm.ContentBlock{llm.ToolResultBlock{
			ToolUseID: id,
			Content:   interruptedToolResultContent,
			IsError:   true,
		}},
	}
}
