package llm

import (
	"errors"
	"fmt"
	"strings"
)

// APIError is returned when DeepSeek's API replies with a non-2xx
// status. The Body field carries the raw response so callers can
// surface it to the user (often the most actionable signal).
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("deepseek api error: status=%d body=%s", e.Status, e.Body)
}

// IsRateLimit reports whether the error indicates a rate-limit response.
func IsRateLimit(err error) bool {
	a, ok := err.(*APIError)
	return ok && a.Status == 429
}

// IsAuth reports whether the error indicates an authentication failure.
func IsAuth(err error) bool {
	a, ok := err.(*APIError)
	return ok && (a.Status == 401 || a.Status == 403)
}

// IsTransient reports whether the error is retryable: rate-limit or
// server-side (5xx) errors. Auth errors (401/403) and client errors
// (400) are NOT transient — retrying them wastes time.
func IsTransient(err error) bool {
	a, ok := err.(*APIError)
	if !ok {
		return false
	}
	return a.Status == 429 || a.Status >= 500
}

// IsContextOverflow reports whether err is a provider rejection for exceeding
// the model's context window. DeepSeek signals this with HTTP 400 and a body
// naming the maximum context length; there is no dedicated error code, so the
// body is matched case-insensitively. Such a request can never succeed on a
// blind retry — the only recovery is to shrink the conversation (compaction),
// which is why the agent loop branches on this distinctly from IsTransient.
func IsContextOverflow(err error) bool {
	var a *APIError
	if !errors.As(err, &a) || a.Status != 400 {
		return false
	}
	b := strings.ToLower(a.Body)
	for _, marker := range []string{
		"context length",
		"maximum context",
		"context_length_exceeded",
		"too many tokens",
		"reduce the length",
		"maximum length",
	} {
		if strings.Contains(b, marker) {
			return true
		}
	}
	return false
}
