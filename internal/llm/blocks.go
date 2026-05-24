package llm

import "encoding/json"

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
// Concrete types live in this same file.
type ContentBlock interface {
	BlockKind() BlockKind
}

// TextBlock is plain assistant or user text.
type TextBlock struct {
	Text string `json:"text"`
}

// ThinkingBlock carries DeepSeek reasoning_content. Signature is
// kept for forward compatibility with Anthropic-style endpoints; the
// DeepSeek API does not populate it.
type ThinkingBlock struct {
	Text      string `json:"text"`
	Signature string `json:"signature,omitempty"`
}

// ToolUseBlock is the model's request to invoke a tool. Input holds
// the raw JSON arguments so we can echo bytes verbatim into the wire
// flatten path without re-marshaling (cache-stable).
type ToolUseBlock struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// ToolResultBlock is the executor's reply to a tool_use. IsError
// surfaces infrastructure failures (tool returned error) versus
// model-visible content; the wire layer always sends Content but
// upstream consumers may color it differently.
type ToolResultBlock struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

func (TextBlock) BlockKind() BlockKind       { return BlockKindText }
func (ThinkingBlock) BlockKind() BlockKind   { return BlockKindThinking }
func (ToolUseBlock) BlockKind() BlockKind    { return BlockKindToolUse }
func (ToolResultBlock) BlockKind() BlockKind { return BlockKindToolResult }
