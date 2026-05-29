package llm

import (
	"bytes"
	"encoding/json"
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
	TopP           *float64       `json:"top_p,omitempty"`
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
//
// Blocks is the canonical representation. The wire flatten layer
// (Request.MarshalCacheStable → flattenForWire) folds Blocks back
// into DeepSeek's OpenAI-shape {content, reasoning_content,
// tool_calls} fields per request, so reasoning_content still
// round-trips for thinking mode.
type Message struct {
	Role string `json:"role"`

	// Blocks is not marshaled here — Request.MarshalCacheStable
	// flattens it via blocks_marshal.go.
	Blocks []ContentBlock `json:"-"`

	Name string `json:"name,omitempty"`
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
// canonicalized by key-sort, Messages flattened from ContentBlock form
// into DeepSeek's OpenAI shape. Pass this to the HTTP layer instead of
// json.Marshal directly.
//
// The request is first sanitized for DeepSeek thinking mode: assistant
// messages with tool calls but no thinking block have a placeholder
// reasoning_content prepended.
func (r Request) MarshalCacheStable() ([]byte, error) {
	// Sanitize first so thinking-mode replay is stable
	r = r.SanitizeForDeepSeek()
	// canonicalizeTools is the single shared canonicalization with the Prefix
	// Fingerprint (static_prefix.go), so the tool bytes on the wire and the
	// bytes the fingerprint hashes are produced by the same code.
	tools, err := canonicalizeTools(r.Tools)
	if err != nil {
		return nil, err
	}

	wireMsgs := make([]wireMessage, 0, len(r.Messages))
	for _, m := range r.Messages {
		results, remainder := splitToolResults(m)
		// Emit any tool-result envelopes first so they line up after
		// the assistant tool_calls turn that produced them — DeepSeek
		// matches by tool_call_id but ordering keeps logs readable.
		wireMsgs = append(wireMsgs, results...)
		if len(remainder.Blocks) > 0 || len(results) == 0 {
			wireMsgs = append(wireMsgs, flattenForWire(remainder))
		}
	}

	out := struct {
		Model          string           `json:"model"`
		Messages       []wireMessage    `json:"messages"`
		Stream         bool             `json:"stream"`
		StreamOptions  *StreamOptions   `json:"stream_options,omitempty"`
		Tools          []Tool           `json:"tools,omitempty"`
		ToolChoice     string           `json:"tool_choice,omitempty"`
		Temperature    *float64         `json:"temperature,omitempty"`
		TopP           *float64         `json:"top_p,omitempty"`
		MaxTokens      int              `json:"max_tokens,omitempty"`
		ResponseFormat *ResponseFmt     `json:"response_format,omitempty"`
		Thinking       *ThinkingOptions `json:"thinking,omitempty"`
	}{
		Model:          r.Model,
		Messages:       wireMsgs,
		Stream:         r.Stream,
		StreamOptions:  r.StreamOptions,
		Tools:          tools,
		ToolChoice:     r.ToolChoice,
		Temperature:    r.Temperature,
		TopP:           r.TopP,
		MaxTokens:      r.MaxTokens,
		ResponseFormat: r.ResponseFormat,
		Thinking:       r.Thinking,
	}
	return json.Marshal(out)
}

// splitToolResults pulls every ToolResultBlock out of m.Blocks and
// returns one wireMessage per result (role="tool"). The returned
// remainder Message keeps m's Role/Name/legacy-fields and a Blocks
// slice trimmed to the non-result subset. Tool-result wireMessages
// are emitted in source order to keep request logs readable.
func splitToolResults(m Message) (results []wireMessage, remainder Message) {
	remainder = m
	if len(m.Blocks) == 0 {
		return nil, remainder
	}
	rest := make([]ContentBlock, 0, len(m.Blocks))
	for _, b := range m.Blocks {
		if tr, ok := b.(ToolResultBlock); ok {
			results = append(results, wireMessage{
				Role:       "tool",
				ToolCallID: tr.ToolUseID,
				Content:    tr.Content,
			})
			continue
		}
		rest = append(rest, b)
	}
	remainder.Blocks = rest
	return results, remainder
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
