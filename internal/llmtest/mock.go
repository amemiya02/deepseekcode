// Package llmtest provides a deterministic, offline mock DeepSeek server
// for exercising the agent loop and the llm client end-to-end without a
// network connection or API credentials.
//
// The seam it plugs into is the same one production uses: llm.Client talks
// to whatever BaseURL it is given, so pointing a Client at this package's
// httptest.Server reproduces the real wire path — SSE framing, the
// reasoning_content channel, tool_call deltas, the finish-reason override,
// the two-tier stream timeout, and the trailing usage frame — with no
// third-party dependency (net/http/httptest is stdlib).
//
// A run is scripted as an ordered list of Turn values, one per model
// round-trip (one Client.Stream call). The server records every request
// body it receives so tests can assert on what was actually sent (e.g. that
// `thinking` serialized as a struct, or that a tool_call paired with its
// result on the following turn). When a run issues more requests than there
// are scripted Turns, the server replies with a synthetic terminal "stop"
// turn so an under-scripted test can never spin the loop to its step cap.
//
// Import note: this package imports internal/llm, so internal/llm's own
// tests cannot import it (that would be an import cycle). It is meant for
// internal/agent, cmd/dsc, and other callers above the llm layer.
package llmtest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// ToolCall is one tool invocation a Turn streams to the agent.
type ToolCall struct {
	ID   string
	Name string
	Args string // raw JSON arguments, streamed verbatim
}

// Usage overrides the usage frame emitted at the end of a Turn. The zero
// value of *Usage (nil) yields a realistic cache-hit usage so cache-metric
// and budget code paths see plausible numbers.
type Usage struct {
	Prompt     int
	Completion int
	CacheHit   int
	CacheMiss  int
}

// Turn scripts a single model response — exactly one HTTP round-trip, i.e.
// one llm.Client.Stream call / one agent step.
type Turn struct {
	// Reasoning is emitted on DeepSeek's reasoning_content channel before
	// any visible content (the thinking-mode output).
	Reasoning string
	// Text is the visible assistant content.
	Text string
	// ToolCalls are streamed as tool_call deltas.
	ToolCalls []ToolCall

	// Finish overrides finish_reason. Empty is inferred: "tool_calls" when
	// ToolCalls is non-empty, otherwise "stop". Set Finish to "stop" while
	// also providing ToolCalls to exercise the finish-reason override.
	Finish string

	// Usage overrides the trailing usage frame; nil yields a default
	// cache-hit usage. OmitUsage suppresses the frame entirely.
	Usage     *Usage
	OmitUsage bool

	// --- fault injection ---

	// Status, when non-zero, returns this HTTP status code (with Body) in
	// place of a stream — for error/retry-path tests.
	Status int
	Body   string

	// DelayBeforeHeaders delays before the response headers are sent, so the
	// client's HTTP round-trip (Client.Stream → Do) is still blocked waiting
	// for headers when a short request-context deadline or cancellation fires.
	// Context abort at this connect/headers phase is reliable (Do honours the
	// request context), unlike a context deadline that fires mid-body. Use it
	// to exercise per-step-deadline and user-cancellation paths.
	DelayBeforeHeaders time.Duration

	// DelayFirst delays the first SSE chunk (after headers), to trip the
	// client's first-token timeout. DelayChunk delays every chunk after the
	// first, to trip the chunk-stall timeout.
	DelayFirst time.Duration
	DelayChunk time.Duration

	// OmitDone suppresses the trailing "data: [DONE]" sentinel, modelling a
	// truncated stream.
	OmitDone bool

	// MalformedTail emits a malformed SSE data line after the content chunks
	// (reasoning/text/tool calls) and before any finish or usage frame. The
	// client accumulates the content it already received, then fails parsing
	// the bad line — modelling a mid-stream break with partial content
	// already in hand (the case partial-turn persistence must survive).
	MalformedTail bool
}

// Server is a scripted mock DeepSeek endpoint. Construct it with NewServer
// and close it with Close (embedded from httptest.Server).
type Server struct {
	*httptest.Server

	mu       sync.Mutex
	turns    []Turn
	served   int
	requests [][]byte
}

// NewServer starts a mock server that serves the given turns in order.
func NewServer(turns ...Turn) *Server {
	s := &Server{turns: turns}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	return s
}

// Client returns an llm.Client pointed at this server with test-friendly
// timeouts and retries disabled. Callers may mutate the returned Client's
// FirstTokenTimeout / ChunkStallTimeout / MaxRetries to drive timeout and
// retry tests.
func (s *Server) Client() *llm.Client {
	c := llm.NewClient("test-key", s.URL)
	c.FirstTokenTimeout = 2 * time.Second
	c.ChunkStallTimeout = 2 * time.Second
	c.MaxRetries = 0
	return c
}

// Requests returns a copy of every request body received so far, in order.
func (s *Server) Requests() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([][]byte, len(s.requests))
	copy(out, s.requests)
	return out
}

// LastRequest returns the most recent request body, or nil if none.
func (s *Server) LastRequest() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.requests) == 0 {
		return nil
	}
	return s.requests[len(s.requests)-1]
}

// Count returns how many requests the server has served.
func (s *Server) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()

	s.mu.Lock()
	s.requests = append(s.requests, body)
	idx := s.served
	s.served++
	var turn Turn
	if idx < len(s.turns) {
		turn = s.turns[idx]
	} else {
		// Synthetic terminal turn: an under-scripted run stops cleanly
		// instead of looping to the agent's step cap.
		turn = Turn{Text: "(llmtest: no scripted turn)"}
	}
	s.mu.Unlock()

	if turn.Status != 0 {
		w.WriteHeader(turn.Status)
		_, _ = io.WriteString(w, turn.Body)
		return
	}

	// Delay before headers: leaves the client blocked in its HTTP round-trip,
	// so a short request-context deadline/cancel aborts at the connect phase.
	if turn.DelayBeforeHeaders > 0 {
		time.Sleep(turn.DelayBeforeHeaders)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		panic("llmtest: ResponseWriter does not implement http.Flusher")
	}

	// Flush the response headers before any DelayFirst sleep. The client's
	// first-token timer starts once it has the response headers, so the
	// delay must sit between headers and the first SSE event — not before
	// the headers, where it would merely block the initial HTTP round-trip.
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Delays use a plain sleep, which holds the connection open for the whole
	// delay. That is deliberate: at the instant a client timeout fires, the
	// response has produced no end-of-stream, so the client observes only its
	// own timeout and can never read a clean (empty) stream-end and misreport
	// the turn as finished. The cost is that Server.Close blocks until the
	// delay elapses, so tests keep delays modest but comfortably larger than
	// the timeout they exercise.
	if turn.DelayFirst > 0 {
		time.Sleep(turn.DelayFirst)
	}

	// reasoning_content first (model thinks before it speaks).
	if turn.Reasoning != "" {
		s.writeChunk(w, flusher, chunk{Choices: []choice{{Delta: delta{ReasoningContent: turn.Reasoning}}}})
	}

	// Then either tool calls or visible text.
	if len(turn.ToolCalls) > 0 {
		for i, tc := range turn.ToolCalls {
			if turn.DelayChunk > 0 {
				time.Sleep(turn.DelayChunk)
			}
			s.writeChunk(w, flusher, chunk{Choices: []choice{{Delta: delta{
				ToolCalls: []wireToolCall{{
					Index:    i,
					ID:       tc.ID,
					Type:     "function",
					Function: wireFunc{Name: tc.Name, Arguments: tc.Args},
				}},
			}}}})
		}
	} else if turn.Text != "" {
		if turn.DelayChunk > 0 {
			time.Sleep(turn.DelayChunk)
		}
		s.writeChunk(w, flusher, chunk{Choices: []choice{{Delta: delta{Content: turn.Text}}}})
	}

	if turn.MalformedTail {
		_, _ = fmt.Fprint(w, "data: {not-valid-json\n\n")
		flusher.Flush()
		return
	}

	finish := turn.Finish
	if finish == "" {
		if len(turn.ToolCalls) > 0 {
			finish = "tool_calls"
		} else {
			finish = "stop"
		}
	}
	if turn.DelayChunk > 0 {
		time.Sleep(turn.DelayChunk)
	}
	s.writeChunk(w, flusher, chunk{Choices: []choice{{FinishReason: finish, Delta: delta{}}}})

	if !turn.OmitUsage {
		u := turn.Usage
		if u == nil {
			u = &Usage{Prompt: 1000, Completion: 50, CacheHit: 900, CacheMiss: 100}
		}
		s.writeChunk(w, flusher, chunk{Usage: &wireUsage{
			PromptTokens:          u.Prompt,
			CompletionTokens:      u.Completion,
			TotalTokens:           u.Prompt + u.Completion,
			PromptCacheHitTokens:  u.CacheHit,
			PromptCacheMissTokens: u.CacheMiss,
		}})
	}

	if !turn.OmitDone {
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}
}

func (s *Server) writeChunk(w http.ResponseWriter, f http.Flusher, c chunk) {
	b, _ := json.Marshal(c)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
	f.Flush()
}

// --- SSE wire shapes (output side; OpenAI-compatible chat.completion.chunk) ---

type chunk struct {
	ID      string     `json:"id,omitempty"`
	Object  string     `json:"object,omitempty"`
	Created int64      `json:"created,omitempty"`
	Model   string     `json:"model,omitempty"`
	Choices []choice   `json:"choices,omitempty"`
	Usage   *wireUsage `json:"usage,omitempty"`
}

type choice struct {
	Index        int    `json:"index"`
	Delta        delta  `json:"delta"`
	FinishReason string `json:"finish_reason,omitempty"`
}

type delta struct {
	Role             string         `json:"role,omitempty"`
	Content          string         `json:"content,omitempty"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
	ToolCalls        []wireToolCall `json:"tool_calls,omitempty"`
}

type wireToolCall struct {
	Index    int      `json:"index"`
	ID       string   `json:"id,omitempty"`
	Type     string   `json:"type,omitempty"`
	Function wireFunc `json:"function"`
}

type wireFunc struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type wireUsage struct {
	PromptTokens          int `json:"prompt_tokens"`
	CompletionTokens      int `json:"completion_tokens"`
	TotalTokens           int `json:"total_tokens"`
	PromptCacheHitTokens  int `json:"prompt_cache_hit_tokens"`
	PromptCacheMissTokens int `json:"prompt_cache_miss_tokens"`
}
