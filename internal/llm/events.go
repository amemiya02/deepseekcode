// Package llm is a hand-rolled DeepSeek API client.
//
// The wire protocol is OpenAI-compatible function calling over SSE.
// We emit a typed event channel (one EventType per chunk class) so the
// agent loop can drive its callback table directly off the channel.
//
// Why hand-rolled: every DeepSeek-specific quirk (cache hit metrics,
// reasoning_content as a separate channel, the finish-reason override
// for "stop while emitting tool_calls", thinking-mode toggling, the
// two-tier stream timeout) is awkward in third-party Go OpenAI clients.
// The whole client is ~400 LOC. Direct control wins.
package llm

import "encoding/json"

// EventType enumerates the kinds of stream events the client emits.
type EventType int

const (
	// EventTextDelta carries an incremental delta of the assistant's
	// visible message content.
	EventTextDelta EventType = iota + 1

	// EventReasoningDelta carries an incremental delta of the
	// assistant's reasoning_content channel (thinking-mode output).
	EventReasoningDelta

	// EventToolCallDelta carries a partial tool_call. Index identifies
	// which tool_call (multiple may be streamed concurrently). Name and
	// Arguments fields accumulate across deltas.
	EventToolCallDelta

	// EventFinish is emitted exactly once at stream end. It carries the
	// finish reason, full Usage block, and the final assembled
	// ToolCalls. The agent loop applies the finish-reason override here:
	// if FinishReason == "stop" but len(ToolCalls) > 0, the loop keeps
	// looping. See docs/design.md §6.4.
	EventFinish

	// EventError indicates a non-recoverable error; the caller should
	// abandon the stream. Recoverable transient errors are surfaced via
	// the returned `error` from Client.Stream() instead.
	EventError
)

// Event is the discriminated union sent on the stream channel.
type Event struct {
	Type EventType

	// EventTextDelta / EventReasoningDelta
	Text string

	// EventToolCallDelta
	ToolCallIndex int
	ToolCallID    string
	ToolName      string
	ArgsDelta     string // appended to a per-index buffer

	// EventFinish
	FinishReason string
	Usage        Usage
	ToolCalls    []ToolCall

	// EventError
	Err error
}

// Usage mirrors the OpenAI-shaped usage block, with DeepSeek's cache
// hit/miss fields. Yuan cost is computed by cache_metrics.go after
// the fact since prices change.
type Usage struct {
	PromptTokens          int `json:"prompt_tokens"`
	CompletionTokens      int `json:"completion_tokens"`
	TotalTokens           int `json:"total_tokens"`
	PromptCacheHitTokens  int `json:"prompt_cache_hit_tokens"`
	PromptCacheMissTokens int `json:"prompt_cache_miss_tokens"`
	// ReasoningTokens is reported by DeepSeek's reasoning models.
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
}

// ToolCall is the fully-assembled tool call (after all deltas arrive).
type ToolCall struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"` // always "function" today
	Function  ToolCallFunc    `json:"function"`
	RawArgs   string          `json:"-"` // for cache-stable rehydration
	Arguments json.RawMessage `json:"-"` // parsed JSON of Function.Arguments
}

// ToolCallFunc is the function-call sub-object.
type ToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
