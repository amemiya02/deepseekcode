package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
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
// It enforces the same two-tier timeout semantics as client.go readSSE:
// FirstTokenTimeout caps the wait until the first SSE event, and
// ChunkStallTimeout caps the gap between subsequent events.
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
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		return nil, &APIError{Status: resp.StatusCode, Body: string(buf)}
	}

	ch := make(chan Event, 32)
	go p.readOpenAINativeSSE(ctx, resp.Body, ch)
	return ch, nil
}

// readOpenAINativeSSE consumes an OpenAI SSE stream and dispatches typed Events.
// It enforces a two-tier timeout (first-token vs chunk-stall) matching client.go.
// EventFinish is emitted exactly once, from the [DONE] sentinel.
func (p *OpenAINativeProvider) readOpenAINativeSSE(ctx context.Context, body io.ReadCloser, out chan<- Event) {
	defer close(out)
	defer func() { _ = body.Close() }()

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	scanner.Split(bufio.ScanLines)

	var (
		finishReason  string
		seenFirstByte bool
	)

	// Two-tier timeout mirroring client.go readSSE.
	firstTimer := time.NewTimer(p.client.FirstTokenTimeout)
	stallTimer := time.NewTimer(p.client.ChunkStallTimeout)
	stallTimer.Stop() // not started until first byte arrives

	defer firstTimer.Stop()
	defer stallTimer.Stop()

	type scanResult struct {
		line string
		ok   bool
		err  error
	}
	lines := make(chan scanResult, 1)
	go func() {
		for scanner.Scan() {
			lines <- scanResult{line: scanner.Text(), ok: true}
		}
		lines <- scanResult{ok: false, err: scanner.Err()}
		close(lines)
	}()

	emit := func(e Event) bool {
		select {
		case out <- e:
			return true
		case <-ctx.Done():
			return false
		}
	}

	for {
		var sr scanResult
		select {
		case <-ctx.Done():
			emit(Event{Type: EventError, Err: ctx.Err()})
			return
		case <-firstTimer.C:
			if !seenFirstByte {
				emit(Event{Type: EventError, Err: fmt.Errorf("%w after %s", ErrFirstTokenTimeout, p.client.FirstTokenTimeout)})
				return
			}
		case <-stallTimer.C:
			emit(Event{Type: EventError, Err: fmt.Errorf("%w after %s", ErrChunkStall, p.client.ChunkStallTimeout)})
			return
		case sr = <-lines:
		}

		if !sr.ok {
			if sr.err != nil {
				emit(Event{Type: EventError, Err: sr.err})
				return
			}
			break
		}

		if !seenFirstByte {
			seenFirstByte = true
			firstTimer.Stop()
		}
		// Reset stall timer on every received line.
		if !stallTimer.Stop() {
			select {
			case <-stallTimer.C:
			default:
			}
		}
		stallTimer.Reset(p.client.ChunkStallTimeout)

		line := strings.TrimSpace(sr.line)
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			// Emit EventFinish exactly once here, not in parseOpenAINativeSSEData.
			emit(Event{Type: EventFinish, FinishReason: finishReason})
			return
		}
		ev := parseOpenAINativeSSEData(data)
		if ev != nil {
			if ev.Type == EventFinish {
				// Capture finish_reason but do NOT emit yet; [DONE] will emit.
				finishReason = ev.FinishReason
			} else {
				if !emit(*ev) {
					return
				}
			}
		}
	}

	// Stream ended without [DONE] (connection closed). Emit finish with
	// whatever finish_reason was collected, matching client.go behaviour.
	emit(Event{Type: EventFinish, FinishReason: finishReason})
}

// parseOpenAINativeSSEData decodes one OpenAI SSE data line.
// It returns EventTextDelta for content deltas, and EventFinish (FinishReason
// only, not to be emitted directly) when finish_reason=="stop" is seen.
// The caller (readOpenAINativeSSE) controls when EventFinish is actually sent.
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
	if c.FinishReason != nil && *c.FinishReason != "" {
		return &Event{Type: EventFinish, FinishReason: *c.FinishReason}
	}
	return nil
}
