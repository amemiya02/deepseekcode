package llm

import (
	"encoding/json"
	"fmt"
)

// ── Wire types (Anthropic Messages API) ───────────────────────────────────────

type anthropicCacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

type anthropicContentBlock struct {
	Type         string                 `json:"type"`                    // "text"
	Text         string                 `json:"text"`
	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

type anthropicMessage struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

type anthropicSystemBlock struct {
	Type         string                 `json:"type"` // "text"
	Text         string                 `json:"text"`
	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

type anthropicWireRequest struct {
	Model       string                 `json:"model"`
	System      []anthropicSystemBlock `json:"system,omitempty"`
	Messages    []anthropicMessage     `json:"messages"`
	MaxTokens   int                    `json:"max_tokens"`
	Temperature float64                `json:"temperature,omitempty"`
}

// textFromBlocks extracts all TextBlock text values from a Message's Blocks,
// concatenating them into a single string. Non-text blocks are skipped.
func textFromBlocks(blocks []ContentBlock) string {
	var out string
	for _, b := range blocks {
		if tb, ok := b.(TextBlock); ok {
			out += tb.Text
		}
	}
	return out
}

// anthropicMarshal converts a provider-neutral Request into Anthropic Messages
// API wire bytes.
//
// Rules:
//   - system messages are lifted to the top-level "system" array; the last
//     system block gets cache_control:{type:ephemeral}.
//   - non-system messages become "messages"; the last user turn's last content
//     block gets cache_control:{type:ephemeral}.
//   - "stream" is NOT included in the body (passed via Accept/stream header).
//   - "max_tokens" is required; defaults to 1024 if Request.MaxTokens == 0.
func anthropicMarshal(req Request) ([]byte, error) {
	var sysBlocks []anthropicSystemBlock
	var msgs []anthropicMessage

	for _, m := range req.Messages {
		text := textFromBlocks(m.Blocks)
		if m.Role == "system" {
			sysBlocks = append(sysBlocks, anthropicSystemBlock{
				Type: "text",
				Text: text,
			})
			continue
		}
		msgs = append(msgs, anthropicMessage{
			Role: m.Role,
			Content: []anthropicContentBlock{
				{Type: "text", Text: text},
			},
		})
	}

	// Apply cache_control to the last system block
	if len(sysBlocks) > 0 {
		sysBlocks[len(sysBlocks)-1].CacheControl = &anthropicCacheControl{Type: "ephemeral"}
	}

	// Apply cache_control to the last user turn's last content block
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			blocks := msgs[i].Content
			if len(blocks) > 0 {
				blocks[len(blocks)-1].CacheControl = &anthropicCacheControl{Type: "ephemeral"}
				msgs[i].Content = blocks
			}
			break
		}
	}

	maxTok := req.MaxTokens
	if maxTok == 0 {
		maxTok = 1024
	}

	var temp float64
	if req.Temperature != nil {
		temp = *req.Temperature
	}

	wire := anthropicWireRequest{
		Model:       req.Model,
		System:      sysBlocks,
		Messages:    msgs,
		MaxTokens:   maxTok,
		Temperature: temp,
	}

	b, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("anthropicMarshal: %w", err)
	}
	return b, nil
}
