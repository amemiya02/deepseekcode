package session

import (
	"encoding/json"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// rebuildBlocksFromLegacy synthesizes a Blocks slice from the
// pre-v2 columns. Used when LoadMessages encounters a row written
// before the v2 migration: the blocks column is empty but the
// legacy content/reasoning/tool_calls columns carry the same
// information in flattened form.
//
// Ordering matches the wire emission order DeepSeek uses, so a
// downstream MarshalCacheStable round-trip produces the same bytes
// regardless of source path:
//
//	Thinking → Text → ToolUse (assistant role only)
//	ToolResult                (tool role)
//	Text                      (user role)
func rebuildBlocksFromLegacy(role, content, reasoning, toolCalls, toolCallID string) []llm.ContentBlock {
	switch role {
	case "assistant":
		var out []llm.ContentBlock
		if reasoning != "" {
			out = append(out, llm.ThinkingBlock{Text: reasoning})
		}
		if content != "" {
			out = append(out, llm.TextBlock{Text: content})
		}
		if toolCalls != "" {
			var calls []llm.ToolCall
			if err := json.Unmarshal([]byte(toolCalls), &calls); err == nil {
				for _, c := range calls {
					input := json.RawMessage(c.Function.Arguments)
					if len(input) == 0 {
						input = json.RawMessage("{}")
					}
					out = append(out, llm.ToolUseBlock{
						ID:    c.ID,
						Name:  c.Function.Name,
						Input: input,
					})
				}
			}
		}
		return out
	case "tool":
		return []llm.ContentBlock{llm.ToolResultBlock{
			ToolUseID: toolCallID,
			Content:   content,
		}}
	case "user", "system":
		if content == "" {
			return nil
		}
		return []llm.ContentBlock{llm.TextBlock{Text: content}}
	default:
		return nil
	}
}
