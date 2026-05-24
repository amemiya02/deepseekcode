package llm

// BlockKind enumerates the kinds of ContentBlock the agent operates on.
// Wire-level flattening in MarshalCacheStable maps each kind to its
// DeepSeek OpenAI-shape field (text → content, thinking →
// reasoning_content, tool_use → tool_calls[], tool_result → a
// separate {role:"tool"} message).
type BlockKind string

const (
	BlockKindText       BlockKind = "text"
	BlockKindThinking   BlockKind = "thinking"
	BlockKindToolUse    BlockKind = "tool_use"
	BlockKindToolResult BlockKind = "tool_result"
)

// ContentBlock is the sealed interface every block type implements.
// Concrete types (TextBlock, ThinkingBlock, ToolUseBlock,
// ToolResultBlock) live in this same file once T-002 lands.
type ContentBlock interface {
	BlockKind() BlockKind
}
