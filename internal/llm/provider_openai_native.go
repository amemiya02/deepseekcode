package llm

import (
	"encoding/json"
	"fmt"
)

// ── Wire types (OpenAI Chat Completions API) ──────────────────────────────────

type openaiNativeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiNativeWireRequest struct {
	Model       string                `json:"model"`
	Messages    []openaiNativeMessage `json:"messages"`
	MaxTokens   int                   `json:"max_tokens"`
	Temperature *float64              `json:"temperature,omitempty"`
	Stream      bool                  `json:"stream"`
}

// openaiNativeMarshal converts a provider-neutral Request to OpenAI Chat
// Completions wire bytes.
//
// Rules:
//   - system messages stay inside "messages" (OpenAI style).
//   - no reasoning_effort, cache_control, or prefix fields emitted.
//   - "stream" is included in the body (OpenAI style, unlike Anthropic).
//   - max_tokens defaults to 1024 if zero.
func openaiNativeMarshal(req Request) ([]byte, error) {
	msgs := make([]openaiNativeMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		content := textFromBlocks(m.Blocks)
		msgs = append(msgs, openaiNativeMessage{Role: m.Role, Content: content})
	}

	maxTok := req.MaxTokens
	if maxTok == 0 {
		maxTok = 1024
	}

	wire := openaiNativeWireRequest{
		Model:       req.Model,
		Messages:    msgs,
		MaxTokens:   maxTok,
		Temperature: req.Temperature,
		Stream:      req.Stream,
	}

	b, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("openaiNativeMarshal: %w", err)
	}
	return b, nil
}
