package llm

import "strings"

// ReasoningEffort controls DeepSeek V4's reasoning depth when thinking
// is enabled. The API accepts "low", "medium", "high", and "max".
// Empty means the effort field is omitted from the wire request.
type ReasoningEffort string

const (
	ReasoningEffortLow    ReasoningEffort = "low"
	ReasoningEffortMedium ReasoningEffort = "medium"
	ReasoningEffortHigh   ReasoningEffort = "high"
	ReasoningEffortMax    ReasoningEffort = "max"
)

// ParseReasoningEffort parses a string into a valid ReasoningEffort.
// It trims surrounding whitespace and accepts lowercase only.
// Returns ("", false) for empty-after-trim or invalid input.
func ParseReasoningEffort(s string) (ReasoningEffort, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	e := ReasoningEffort(s)
	if !e.Valid() {
		return "", false
	}
	return e, true
}

// Valid reports whether e is one of the four recognized effort levels.
func (e ReasoningEffort) Valid() bool {
	switch e {
	case ReasoningEffortLow, ReasoningEffortMedium, ReasoningEffortHigh, ReasoningEffortMax:
		return true
	default:
		return false
	}
}

// String returns the string representation of the effort level.
// For an invalid (empty) effort, returns "".
func (e ReasoningEffort) String() string {
	return string(e)
}
