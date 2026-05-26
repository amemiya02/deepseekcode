package repair

import (
	"github.com/amemiya02/deepseekcode/internal/llm"
)

// ToolKind categorizes tools as read-only or mutating.
type ToolKind int

const (
	// ToolReadOnly indicates a tool that does not modify state.
	ToolReadOnly ToolKind = iota
	// ToolMutating indicates a tool that modifies state.
	ToolMutating
)

// StormBreaker suppresses repeated identical read-only tool calls.
type StormBreaker struct {
	window    int                // number of recent calls to track
	threshold int                // occurrences needed to suppress
	history   []callIdentity     // recent call identities
}

// callIdentity represents a unique call by tool name and canonical arguments.
type callIdentity struct {
	name string
	args string // canonical args or raw args if invalid
}

// NewStormBreaker creates a StormBreaker with the given window and threshold.
// Non-positive values default to window=6 and threshold=3.
func NewStormBreaker(window, threshold int) *StormBreaker {
	if window <= 0 {
		window = 6
	}
	if threshold <= 0 {
		threshold = 3
	}
	return &StormBreaker{
		window:    window,
		threshold: threshold,
		history:   make([]callIdentity, 0, window),
	}
}

// Filter processes a batch of tool calls, suppressing repeated read-only calls.
// It returns the kept calls and reports for suppressed calls.
func (s *StormBreaker) Filter(calls []llm.ToolCall, kinds map[string]ToolKind) Result {
	result := Result{}

	for _, call := range calls {
		kind := kinds[call.Function.Name]
		// Unknown tools are treated as mutating (safe default)
		if kind == ToolReadOnly && kinds != nil {
			if _, exists := kinds[call.Function.Name]; !exists {
				kind = ToolMutating
			}
		}

		// Mutating calls clear history and are never suppressed
		if kind == ToolMutating {
			s.history = s.history[:0]
			result.Calls = append(result.Calls, call)
			continue
		}

		// Build identity for read-only call
		identity := callIdentity{
			name: call.Function.Name,
			args: canonicalOrRaw(call.Function.Arguments),
		}

		// Count occurrences in current window
		count := s.countInWindow(identity)

		// Suppress if this would be the threshold-th identical call
		if count >= s.threshold-1 {
			// Suppress this call
			result.Reports = append(result.Reports, Report{
				Kind:    KindSuppressed,
				Tool:    call.Function.Name,
				CallID:  call.ID,
				Message: "suppressed repeated read-only call",
			})
			continue
		}

		// Keep this call and add to history
		result.Calls = append(result.Calls, call)
		s.addToHistory(identity)
	}

	return result
}

// Reset clears all history.
func (s *StormBreaker) Reset() {
	s.history = s.history[:0]
}

// countInWindow counts occurrences of identity in recent history.
func (s *StormBreaker) countInWindow(identity callIdentity) int {
	count := 0
	start := len(s.history) - s.window
	if start < 0 {
		start = 0
	}
	for i := start; i < len(s.history); i++ {
		if s.history[i] == identity {
			count++
		}
	}
	return count
}

// addToHistory adds an identity to history, trimming to window size.
func (s *StormBreaker) addToHistory(identity callIdentity) {
	s.history = append(s.history, identity)
	// Trim to window size
	if len(s.history) > s.window {
		s.history = s.history[len(s.history)-s.window:]
	}
}

// canonicalOrRaw returns canonical args if valid, otherwise raw args.
func canonicalOrRaw(args string) string {
	canonical, ok := CanonicalArgs(args)
	if !ok {
		return args
	}
	return canonical
}
