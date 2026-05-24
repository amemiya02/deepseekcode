package llm

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// wireMessage is the JSON shape DeepSeek's chat-completions endpoint
// expects. It mirrors the OpenAI Chat Completions message envelope:
// content as a single string, reasoning_content alongside, and any
// tool invocations on tool_calls[]. Tool results travel as separate
// {role:"tool"} envelopes — split out by the caller in T-008, not
// here.
type wireMessage struct {
	Role             string     `json:"role"`
	Content          string     `json:"content,omitempty"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	Name             string     `json:"name,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
}

// flattenForWire converts a Message into the DeepSeek wire shape.
// When Blocks is empty, the legacy Content/ReasoningContent/ToolCalls
// fields pass through unchanged. When Blocks is non-empty, those
// legacy fields are ignored and the wire shape is built from blocks:
//
//   - Multiple TextBlocks concatenate into Content (no separator)
//   - Multiple ThinkingBlocks concatenate into ReasoningContent
//   - ToolUseBlocks become ToolCalls, sorted by ID for cache stability
//   - ToolResultBlocks are NOT handled here — the caller must split
//     them out as separate {role:"tool"} wireMessages
func flattenForWire(m Message) wireMessage {
	out := wireMessage{Role: m.Role, Name: m.Name}
	if len(m.Blocks) == 0 {
		out.Content = m.Content
		out.ReasoningContent = m.ReasoningContent
		out.ToolCalls = m.ToolCalls
		out.ToolCallID = m.ToolCallID
		return out
	}
	var (
		text     strings.Builder
		thinking strings.Builder
		calls    []ToolCall
	)
	for _, b := range m.Blocks {
		switch v := b.(type) {
		case TextBlock:
			text.WriteString(v.Text)
		case ThinkingBlock:
			thinking.WriteString(v.Text)
		case ToolUseBlock:
			args := string(v.Input)
			if args == "" {
				args = "{}"
			}
			calls = append(calls, ToolCall{
				ID:   v.ID,
				Type: "function",
				Function: ToolCallFunc{
					Name:      v.Name,
					Arguments: args,
				},
			})
		case ToolResultBlock:
			// Caller's job (see T-008). Intentionally a no-op here so
			// flattenForWire on a {role:"tool", Blocks:[ToolResult]}
			// produces an empty wireMessage instead of synthesizing
			// the wrong envelope.
		}
	}
	if calls != nil {
		sort.SliceStable(calls, func(i, j int) bool { return calls[i].ID < calls[j].ID })
	}
	out.Content = text.String()
	out.ReasoningContent = thinking.String()
	out.ToolCalls = calls
	return out
}

// MarshalBlocks serializes a slice of ContentBlock to a JSON array.
// Each element carries a "kind" discriminator key followed by the
// concrete type's own fields, inlined.
//
// nil or empty input produces JSON null so roundtrips through
// nullable persistence columns stay symmetric.
func MarshalBlocks(blocks []ContentBlock) ([]byte, error) {
	if len(blocks) == 0 {
		return []byte("null"), nil
	}
	out := make([]json.RawMessage, 0, len(blocks))
	for i, b := range blocks {
		raw, err := marshalOneBlock(b)
		if err != nil {
			return nil, fmt.Errorf("marshal block [%d]: %w", i, err)
		}
		out = append(out, raw)
	}
	return json.Marshal(out)
}

// UnmarshalBlocks parses a JSON array produced by MarshalBlocks back
// into a typed slice. Unknown kinds return an error tagged with the
// element index for fast triage.
//
// A literal "null" or empty array produces (nil, nil) so callers can
// uniformly treat "no blocks" as nil without distinguishing source.
func UnmarshalBlocks(data []byte) ([]ContentBlock, error) {
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		return nil, fmt.Errorf("unmarshal blocks: %w", err)
	}
	if len(raws) == 0 {
		return nil, nil
	}
	out := make([]ContentBlock, 0, len(raws))
	for i, raw := range raws {
		blk, err := unmarshalOneBlock(raw)
		if err != nil {
			return nil, fmt.Errorf("unmarshal block [%d]: %w", i, err)
		}
		out = append(out, blk)
	}
	return out, nil
}

func marshalOneBlock(b ContentBlock) ([]byte, error) {
	switch v := b.(type) {
	case TextBlock:
		return json.Marshal(struct {
			Kind BlockKind `json:"kind"`
			Text string    `json:"text"`
		}{BlockKindText, v.Text})
	case ThinkingBlock:
		return json.Marshal(struct {
			Kind      BlockKind `json:"kind"`
			Text      string    `json:"text"`
			Signature string    `json:"signature,omitempty"`
		}{BlockKindThinking, v.Text, v.Signature})
	case ToolUseBlock:
		input := v.Input
		if len(input) == 0 {
			input = json.RawMessage("{}")
		}
		return json.Marshal(struct {
			Kind  BlockKind       `json:"kind"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		}{BlockKindToolUse, v.ID, v.Name, input})
	case ToolResultBlock:
		return json.Marshal(struct {
			Kind      BlockKind `json:"kind"`
			ToolUseID string    `json:"tool_use_id"`
			Content   string    `json:"content"`
			IsError   bool      `json:"is_error,omitempty"`
		}{BlockKindToolResult, v.ToolUseID, v.Content, v.IsError})
	default:
		return nil, fmt.Errorf("unknown block type: %T", b)
	}
}

func unmarshalOneBlock(raw json.RawMessage) (ContentBlock, error) {
	var probe struct {
		Kind BlockKind `json:"kind"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("read kind: %w", err)
	}
	if probe.Kind == "" {
		return nil, fmt.Errorf("missing kind field")
	}
	switch probe.Kind {
	case BlockKindText:
		var b TextBlock
		if err := json.Unmarshal(raw, &b); err != nil {
			return nil, err
		}
		return b, nil
	case BlockKindThinking:
		var b ThinkingBlock
		if err := json.Unmarshal(raw, &b); err != nil {
			return nil, err
		}
		return b, nil
	case BlockKindToolUse:
		var b ToolUseBlock
		if err := json.Unmarshal(raw, &b); err != nil {
			return nil, err
		}
		return b, nil
	case BlockKindToolResult:
		var b ToolResultBlock
		if err := json.Unmarshal(raw, &b); err != nil {
			return nil, err
		}
		return b, nil
	default:
		return nil, fmt.Errorf("unknown block kind: %q", probe.Kind)
	}
}
