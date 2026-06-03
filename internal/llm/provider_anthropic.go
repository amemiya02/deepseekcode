package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const anthropicAPIVersion = "2023-06-01"

// AnthropicProvider implements Provider for the Anthropic Messages API.
type AnthropicProvider struct {
	client *Client
}

func newAnthropicProvider(cfg ProviderConfig) (Provider, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	c := NewClient(cfg.APIKey, baseURL)
	return &AnthropicProvider{client: c}, nil
}

func (p *AnthropicProvider) Name() string { return "anthropic" }

func (p *AnthropicProvider) BaseClient() *Client { return p.client }

func (p *AnthropicProvider) Capabilities() Capabilities {
	return Capabilities{
		Thinking:             true,
		PrefixCache:          true,
		JSONMode:             false,
		MaxContextTokens:     200_000,
		MaxOutputTokens:      8_192,
		ReasoningEfforts:     nil,
		ChatPrefixCompletion: false,
		FIM:                  false,
		FIMRequiresThinkingOff: false,
		SupportsModels: []string{
			"claude-opus-4-5",
			"claude-sonnet-4-5",
			"claude-haiku-4-5",
		},
	}
}

func (p *AnthropicProvider) ValidatePro(ctx context.Context, prompt string) (bool, string, error) {
	// Stub — Anthropic does not expose a separate validation endpoint.
	return true, "", nil
}

// Stream sends req to the Anthropic Messages API and returns a channel of Events.
// It uses server-sent events (SSE) identical in framing to the DeepSeek path
// but parses Anthropic-specific event types.
func (p *AnthropicProvider) Stream(ctx context.Context, req Request) (<-chan Event, error) {
	body, err := anthropicMarshal(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic stream marshal: %w", err)
	}

	model := req.Model
	if model == "" {
		model = "claude-sonnet-4-5"
	}
	_ = model // used for defaulting; already embedded in marshaled body
	url := strings.TrimRight(p.client.BaseURL, "/") + "/v1/messages"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("anthropic new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.client.APIKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)
	httpReq.Header.Set("anthropic-beta", "prompt-caching-2024-07-31")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.client.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic do: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("anthropic status %d", resp.StatusCode)
	}

	ch := make(chan Event, 32)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		var eventType string
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "event: ") {
				eventType = strings.TrimPrefix(line, "event: ")
				continue
			}
			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")
				ev := parseAnthropicSSEData(eventType, data)
				if ev != nil {
					ch <- *ev
				}
			}
		}
	}()
	return ch, nil
}

// parseAnthropicSSEData converts an Anthropic SSE event+data pair into an Event.
func parseAnthropicSSEData(eventType, data string) *Event {
	var raw map[string]any
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		return nil
	}
	switch eventType {
	case "content_block_delta":
		if delta, ok := raw["delta"].(map[string]any); ok {
			if dt, _ := delta["type"].(string); dt == "text_delta" {
				if text, _ := delta["text"].(string); text != "" {
					// Use EventTextDelta to match the real codebase event types
					return &Event{Type: EventTextDelta, Text: text}
				}
			}
		}
	case "message_delta":
		if delta, ok := raw["delta"].(map[string]any); ok {
			if reason, _ := delta["stop_reason"].(string); reason == "end_turn" {
				// Use EventFinish to match the real codebase event types
				return &Event{Type: EventFinish, FinishReason: "end_turn"}
			}
		}
	case "message_stop":
		return &Event{Type: EventFinish, FinishReason: "end_turn"}
	}
	return nil
}

// ── Wire types (Anthropic Messages API) ───────────────────────────────────────

type anthropicCacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

// anthropicBlock represents a single content block used in both the top-level
// "system" array and per-message "content" arrays. Both positions share the
// same three fields, so a single type serves both roles.
type anthropicBlock struct {
	Type         string                 `json:"type"`                    // "text"
	Text         string                 `json:"text"`
	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

type anthropicMessage struct {
	Role    string           `json:"role"`
	Content []anthropicBlock `json:"content"`
}

type anthropicWireRequest struct {
	Model       string           `json:"model"`
	System      []anthropicBlock `json:"system,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
	MaxTokens   int              `json:"max_tokens"`
	Temperature *float64         `json:"temperature,omitempty"`
}

// textFromBlocks extracts all TextBlock text values from a Message's Blocks,
// concatenating them into a single string. Non-text blocks are skipped.
func textFromBlocks(blocks []ContentBlock) string {
	var sb strings.Builder
	for _, b := range blocks {
		if tb, ok := b.(TextBlock); ok {
			sb.WriteString(tb.Text)
		}
	}
	return sb.String()
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
	var sysBlocks []anthropicBlock
	var msgs []anthropicMessage

	for _, m := range req.Messages {
		text := textFromBlocks(m.Blocks)
		if m.Role == "system" {
			sysBlocks = append(sysBlocks, anthropicBlock{
				Type: "text",
				Text: text,
			})
			continue
		}
		msgs = append(msgs, anthropicMessage{
			Role: m.Role,
			Content: []anthropicBlock{
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

	wire := anthropicWireRequest{
		Model:       req.Model,
		System:      sysBlocks,
		Messages:    msgs,
		MaxTokens:   maxTok,
		Temperature: req.Temperature,
	}

	b, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("anthropicMarshal: %w", err)
	}
	return b, nil
}
