package llm

import "fmt"

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
