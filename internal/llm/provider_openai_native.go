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

// ── OpenAINativeProvider ──────────────────────────────────────────────────────

// OpenAINativeProvider implements Provider for the OpenAI Chat Completions API.
type OpenAINativeProvider struct {
	client *Client
}

func newOpenAINativeProvider(cfg ProviderConfig) (Provider, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	c := NewClient(cfg.APIKey, baseURL)
	applyProviderTimeouts(c, cfg)
	return &OpenAINativeProvider{client: c}, nil
}

func (p *OpenAINativeProvider) Name() string { return "openai" }

func (p *OpenAINativeProvider) BaseClient() *Client { return p.client }

func (p *OpenAINativeProvider) Capabilities() Capabilities {
	return Capabilities{
		Thinking:               false,
		PrefixCache:            false,
		JSONMode:               true,
		MaxContextTokens:       128_000,
		MaxOutputTokens:        4_096,
		ReasoningEfforts:       nil,
		ChatPrefixCompletion:   false,
		FIM:                    false,
		FIMRequiresThinkingOff: false,
		SupportsModels: []string{
			"gpt-4o",
			"gpt-4o-mini",
			"gpt-4-turbo",
		},
	}
}

func (p *OpenAINativeProvider) ValidatePro(_ context.Context, _ string) (bool, string, error) {
	return true, "", nil
}

// Stream sends req to the OpenAI Chat Completions endpoint and returns Events.
func (p *OpenAINativeProvider) Stream(ctx context.Context, req Request) (<-chan Event, error) {
	body, err := openaiNativeMarshal(req)
	if err != nil {
		return nil, fmt.Errorf("openai native stream marshal: %w", err)
	}

	url := strings.TrimRight(p.client.BaseURL, "/") + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai native new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.client.APIKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.client.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai native do: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("openai native status %d", resp.StatusCode)
	}

	ch := make(chan Event, 32)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				ch <- Event{Type: EventFinish, FinishReason: "stop"}
				return
			}
			ev := parseOpenAINativeSSEData(data)
			if ev != nil {
				ch <- *ev
			}
		}
	}()
	return ch, nil
}

// parseOpenAINativeSSEData decodes one OpenAI SSE data line.
func parseOpenAINativeSSEData(data string) *Event {
	var raw struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		return nil
	}
	if len(raw.Choices) == 0 {
		return nil
	}
	c := raw.Choices[0]
	if c.Delta.Content != "" {
		return &Event{Type: EventTextDelta, Text: c.Delta.Content}
	}
	if c.FinishReason != nil && *c.FinishReason == "stop" {
		return &Event{Type: EventFinish, FinishReason: "stop"}
	}
	return nil
}
