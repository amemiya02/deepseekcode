package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

// Client is a thin HTTP+SSE wrapper for DeepSeek's chat completions API.
type Client struct {
	HTTPClient *http.Client
	APIKey     string
	BaseURL    string

	// FirstTokenTimeout caps the wait from request send to the first
	// SSE event. Reasoner cold starts can be 30s+; default 45s.
	FirstTokenTimeout time.Duration

	// ChunkStallTimeout caps the gap between SSE events once streaming
	// has begun. Default 20s.
	ChunkStallTimeout time.Duration

	// MaxRetries is the maximum number of retries for transient errors
	// (429 rate-limit, 5xx server errors). Default 3. 0 disables retry.
	MaxRetries int

	// OnRetry, if non-nil, is called before each retry sleep with the
	// attempt number (1-based) and the error that triggered it.
	OnRetry func(attempt int, err error)
}

// NewClient returns a Client with sensible defaults.
func NewClient(apiKey, baseURL string) *Client {
	return &Client{
		HTTPClient:        &http.Client{Timeout: 0}, // streaming: no global timeout
		APIKey:            apiKey,
		BaseURL:           strings.TrimRight(baseURL, "/"),
		FirstTokenTimeout: 45 * time.Second,
		ChunkStallTimeout: 20 * time.Second,
		MaxRetries:        3,
	}
}

// Stream sends the request and returns a channel of Events plus an
// error if the initial connect or auth fails. Once Stream returns,
// the channel is closed by the goroutine that reads the SSE body.
//
// Caller is expected to drain the channel. To abort, cancel ctx.
//
// Transient errors (429, 5xx) are retried with exponential backoff +
// jitter up to c.MaxRetries times. Non-transient errors (400, 401,
// 403) are returned immediately.
func (c *Client) Stream(ctx context.Context, req Request) (<-chan Event, error) {
	req.Stream = true
	if req.StreamOptions == nil {
		req.StreamOptions = &StreamOptions{IncludeUsage: true}
	}

	body, err := req.MarshalCacheStable()
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	maxRetries := c.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 0 // explicit no-retry
	}

	for attempt := 0; ; attempt++ {
		ch, err := c.doStream(ctx, body)
		if err == nil {
			return ch, nil
		}

		// Non-transient or exhausted retries: fail immediately.
		if !IsTransient(err) || attempt >= maxRetries {
			return nil, err
		}

		if c.OnRetry != nil {
			c.OnRetry(attempt+1, err)
		}

		// Exponential backoff: 1s, 2s, 4s ... capped at 30s, with jitter.
		delay := time.Second << uint(attempt)
		if delay > 30*time.Second {
			delay = 30 * time.Second
		}
		jitter := time.Duration(rand.Int63n(int64(delay) / 2)) // 0..delay/2
		delay += jitter

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
}

// doStream performs a single HTTP+SSE request attempt.
func (c *Client) doStream(ctx context.Context, body []byte) (<-chan Event, error) {
	url := c.BaseURL + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		return nil, &APIError{Status: resp.StatusCode, Body: string(buf)}
	}

	ch := make(chan Event, 32)
	go c.readSSE(ctx, resp.Body, ch)
	return ch, nil
}

// readSSE consumes the server-sent-events stream and dispatches typed
// Events. It enforces the two-tier timeout (first-token vs chunk-stall).
// It accumulates tool_call deltas by index so EventFinish can carry the
// fully-assembled ToolCalls slice.
func (c *Client) readSSE(ctx context.Context, body io.ReadCloser, out chan<- Event) {
	defer close(out)
	defer func() { _ = body.Close() }()

	// We use bufio.Scanner with a generous max line size — JSON-Schema
	// tool descriptions occasionally exceed 64KB.
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	scanner.Split(bufio.ScanLines)

	// State across deltas.
	var (
		toolBuf       = map[int]*toolAcc{} // index → accumulator
		finishReason  string
		usage         Usage
		seenFirstByte bool
	)

	// Two-tier timeout: first-token timer cancels if no progress at all.
	// chunk-stall timer is reset on every event.
	firstTimer := time.NewTimer(c.FirstTokenTimeout)
	stallTimer := time.NewTimer(c.ChunkStallTimeout)
	stallTimer.Stop() // start it on first byte

	defer firstTimer.Stop()
	defer stallTimer.Stop()

	// We read in a goroutine so we can race timeouts.
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
				emit(Event{Type: EventError, Err: fmt.Errorf("first-token timeout after %s", c.FirstTokenTimeout)})
				return
			}
		case <-stallTimer.C:
			emit(Event{Type: EventError, Err: fmt.Errorf("chunk stall timeout after %s", c.ChunkStallTimeout)})
			return
		case sr = <-lines:
		}

		if !sr.ok {
			if sr.err != nil && !errors.Is(sr.err, io.EOF) {
				emit(Event{Type: EventError, Err: sr.err})
				return
			}
			break
		}

		if !seenFirstByte {
			seenFirstByte = true
			firstTimer.Stop()
		}
		// Reset stall timer.
		if !stallTimer.Stop() {
			select {
			case <-stallTimer.C:
			default:
			}
		}
		stallTimer.Reset(c.ChunkStallTimeout)

		line := strings.TrimSpace(sr.line)
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			emit(Event{Type: EventError, Err: fmt.Errorf("malformed SSE chunk: %w", err)})
			return
		}

		// Usage frame (DeepSeek sends a final chunk with usage filled in
		// when stream_options.include_usage = true).
		if chunk.Usage != nil {
			usage = *chunk.Usage
		}

		for _, choice := range chunk.Choices {
			d := choice.Delta
			if d.Content != "" {
				if !emit(Event{Type: EventTextDelta, Text: d.Content}) {
					return
				}
			}
			if d.ReasoningContent != "" {
				if !emit(Event{Type: EventReasoningDelta, Text: d.ReasoningContent}) {
					return
				}
			}
			for _, tc := range d.ToolCalls {
				idx := tc.Index
				acc, ok := toolBuf[idx]
				if !ok {
					acc = &toolAcc{Index: idx}
					toolBuf[idx] = acc
				}
				if tc.ID != "" {
					acc.ID = tc.ID
				}
				if tc.Type != "" {
					acc.Type = tc.Type
				}
				if tc.Function.Name != "" {
					acc.Name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					acc.Args.WriteString(tc.Function.Arguments)
				}
				if !emit(Event{
					Type:          EventToolCallDelta,
					ToolCallIndex: idx,
					ToolCallID:    acc.ID,
					ToolName:      acc.Name,
					ArgsDelta:     tc.Function.Arguments,
				}) {
					return
				}
			}
			if choice.FinishReason != "" {
				finishReason = choice.FinishReason
			}
		}
	}

	// Assemble final tool calls in index order.
	maxIdx := -1
	for i := range toolBuf {
		if i > maxIdx {
			maxIdx = i
		}
	}
	calls := make([]ToolCall, 0, maxIdx+1)
	for i := 0; i <= maxIdx; i++ {
		acc, ok := toolBuf[i]
		if !ok {
			continue
		}
		args := acc.Args.String()
		call := ToolCall{
			ID:   acc.ID,
			Type: acc.Type,
			Function: ToolCallFunc{
				Name:      acc.Name,
				Arguments: args,
			},
			RawArgs: args,
		}
		if args != "" {
			call.Arguments = json.RawMessage(args)
		}
		calls = append(calls, call)
	}

	emit(Event{
		Type:         EventFinish,
		FinishReason: finishReason,
		Usage:        usage,
		ToolCalls:    calls,
	})
}

type toolAcc struct {
	Index int
	ID    string
	Type  string
	Name  string
	Args  strings.Builder
}

// streamChunk is the OpenAI-shaped SSE chunk envelope.
type streamChunk struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []sseChoice  `json:"choices"`
	Usage   *Usage       `json:"usage,omitempty"`
}

type sseChoice struct {
	Index        int      `json:"index"`
	Delta        sseDelta `json:"delta"`
	FinishReason string   `json:"finish_reason,omitempty"`
}

type sseDelta struct {
	Role             string            `json:"role,omitempty"`
	Content          string            `json:"content,omitempty"`
	ReasoningContent string            `json:"reasoning_content,omitempty"`
	ToolCalls        []sseToolCall     `json:"tool_calls,omitempty"`
}

type sseToolCall struct {
	Index    int          `json:"index"`
	ID       string       `json:"id,omitempty"`
	Type     string       `json:"type,omitempty"`
	Function sseFunction  `json:"function"`
}

type sseFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}
