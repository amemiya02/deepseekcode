package llm

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestIsTransient(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"rate limit", &APIError{Status: 429}, true},
		{"server error 500", &APIError{Status: 500}, true},
		{"server error 503", &APIError{Status: 503}, true},
		{"bad request", &APIError{Status: 400}, false},
		{"unauthorized", &APIError{Status: 401}, false},
		{"forbidden", &APIError{Status: 403}, false},
		{"not found", &APIError{Status: 404}, false},
		{"plain error", errors.New("network blew up"), false},
		{"nil", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsTransient(c.err); got != c.want {
				t.Errorf("IsTransient(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

func TestIsContextOverflow(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"deepseek max context body", &APIError{Status: 400, Body: "This model's maximum context length is 1000000 tokens. However, you requested 1200000 tokens."}, true},
		{"openai-style code", &APIError{Status: 400, Body: `{"error":{"code":"context_length_exceeded"}}`}, true},
		{"too many tokens phrasing", &APIError{Status: 400, Body: "Bad request: too many tokens in prompt"}, true},
		{"reduce the length phrasing", &APIError{Status: 400, Body: "Please reduce the length of the messages."}, true},
		{"unrelated 400", &APIError{Status: 400, Body: "invalid 'thinking' field"}, false},
		{"overflow-ish text but wrong status", &APIError{Status: 500, Body: "maximum context length exceeded"}, false},
		{"rate limit", &APIError{Status: 429, Body: "slow down"}, false},
		{"plain error", errors.New("context length"), false}, // not an APIError
		{"wrapped api error", wrap(&APIError{Status: 400, Body: "maximum context length is 1M"}), true},
		{"nil", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsContextOverflow(c.err); got != c.want {
				t.Errorf("IsContextOverflow(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

// wrap mimics a caller wrapping the connect error with %w so the test pins that
// IsContextOverflow unwraps via errors.As, not a bare type assertion.
func wrap(err error) error { return errCtx{err} }

type errCtx struct{ err error }

func (e errCtx) Error() string { return "stream: " + e.err.Error() }
func (e errCtx) Unwrap() error { return e.err }

func TestStreamTimeoutSentinels(t *testing.T) {
	// readSSE emits these wrapped exactly as below. Pin that the wrapped error
	// both matches the sentinel via errors.Is (so the agent can classify a
	// re-issuable stall) AND keeps a greppable message (so logs and the older
	// string-matching tests still read naturally).
	cases := []struct {
		sentinel error
		wrapped  error
		substr   string
	}{
		{ErrFirstTokenTimeout, fmt.Errorf("%w after %s", ErrFirstTokenTimeout, 45*time.Second), "first-token timeout"},
		{ErrChunkStall, fmt.Errorf("%w after %s", ErrChunkStall, 20*time.Second), "chunk stall"},
	}
	for _, c := range cases {
		if !errors.Is(c.wrapped, c.sentinel) {
			t.Errorf("errors.Is(%q, sentinel) = false, want true", c.wrapped)
		}
		if !strings.Contains(c.wrapped.Error(), c.substr) {
			t.Errorf("message %q does not contain %q", c.wrapped.Error(), c.substr)
		}
		// The two sentinels must be distinct so a chunk-stall is never mistaken
		// for a first-token timeout.
		if errors.Is(c.wrapped, ErrFirstTokenTimeout) && errors.Is(c.wrapped, ErrChunkStall) {
			t.Errorf("%q matches both timeout sentinels", c.wrapped)
		}
	}
}
