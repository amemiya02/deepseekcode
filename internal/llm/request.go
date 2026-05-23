package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// Request is the OpenAI-shaped chat completion request DeepSeek accepts.
//
// Serialization is deterministic: map keys sort, tool schemas sort by
// name, no map iteration order leaks. Cache-stable serialization is
// critical because every byte difference invalidates the API-side
// prompt cache, and the 50–120× cache discount is one of our v0.1
// differentiators. See docs/design.md §5.4.
type Request struct {
	Model          string         `json:"model"`
	Messages       []Message      `json:"messages"`
	Stream         bool           `json:"stream"`
	StreamOptions  *StreamOptions `json:"stream_options,omitempty"`
	Tools          []Tool         `json:"tools,omitempty"`
	ToolChoice     string         `json:"tool_choice,omitempty"`
	Temperature    *float64       `json:"temperature,omitempty"`
	MaxTokens      int            `json:"max_tokens,omitempty"`
	ResponseFormat *ResponseFmt   `json:"response_format,omitempty"`

	// Thinking is DeepSeek's reasoning-mode toggle. On deepseek-v4-flash/pro
	// the API expects an object: {"type":"enabled"|"disabled"}. Use
	// ThinkingEnabled() / nil to set; omitempty drops nil so older aliases
	// see no field at all.
	Thinking *ThinkingOptions `json:"thinking,omitempty"`
}

// ThinkingOptions matches DeepSeek V4's reasoning-mode envelope.
type ThinkingOptions struct {
	Type string `json:"type"` // "enabled" | "disabled"
}

// ThinkingEnabled returns a *ThinkingOptions when on is true, else nil.
// Callers should pass the result directly into Request.Thinking.
func ThinkingEnabled(on bool) *ThinkingOptions {
	if !on {
		return nil
	}
	return &ThinkingOptions{Type: "enabled"}
}

// StreamOptions matches OpenAI's; we always set IncludeUsage so the
// final SSE event carries cache hit/miss tokens.
type StreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// Message is a single turn in the chat. Role is one of:
// "system" | "user" | "assistant" | "tool".
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// Tool is a function-calling tool definition.
type Tool struct {
	Type     string       `json:"type"` // "function"
	Function ToolFunction `json:"function"`
}

// ToolFunction is the function sub-object of a Tool. Parameters is a
// JSON-Schema; we keep it as RawMessage so the caller's marshaler can
// guarantee deterministic key ordering.
type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ResponseFmt forces JSON-mode output. The Duet validator uses this so
// pro's approve/block decisions are structured.
type ResponseFmt struct {
	Type string `json:"type"` // "json_object" or "text"
}

// MarshalCacheStable returns a deterministic JSON encoding of the
// request: tool definitions sorted by function name, JSON-Schema fields
// canonicalized by key-sort. Pass this to the HTTP layer instead of
// json.Marshal directly.
func (r Request) MarshalCacheStable() ([]byte, error) {
	// Sort tools by function name to stabilize prefix bytes.
	tools := make([]Tool, len(r.Tools))
	copy(tools, r.Tools)
	sort.SliceStable(tools, func(i, j int) bool {
		return tools[i].Function.Name < tools[j].Function.Name
	})

	// Canonicalize each tool's Parameters (a JSON-Schema) by re-encoding
	// with sorted keys.
	for i, t := range tools {
		canon, err := canonicalJSON(t.Function.Parameters)
		if err != nil {
			return nil, fmt.Errorf("canonicalizing tool %q parameters: %w", t.Function.Name, err)
		}
		tools[i].Function.Parameters = canon
	}

	r.Tools = tools
	return json.Marshal(r)
}

// canonicalJSON re-encodes a JSON document with object keys sorted at
// every level. Stable across runs; cheap.
func canonicalJSON(in json.RawMessage) (json.RawMessage, error) {
	if len(in) == 0 {
		return in, nil
	}
	var v any
	if err := json.Unmarshal(in, &v); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := writeCanonical(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeCanonical(buf *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			kb, err := json.Marshal(k)
			if err != nil {
				return err
			}
			buf.Write(kb)
			buf.WriteByte(':')
			if err := writeCanonical(buf, t[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	case []any:
		buf.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonical(buf, e); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return err
		}
		buf.Write(b)
	}
	return nil
}
