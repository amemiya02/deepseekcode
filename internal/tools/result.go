package tools

import "fmt"

// Result is what a tool returns to the model after Execute.
// Content is the text the model will see in the tool-result message.
// IsError flips the rendering and signals "the tool ran but failed."
type Result struct {
	Content string
	IsError bool
}

// Errf is a convenience for "tool ran and produced an error message
// the model should read." Use Go's standard `error` return value for
// infrastructure failures the agent should retry or surface to the user.
func Errf(format string, args ...any) Result {
	return Result{Content: sprintf(format, args...), IsError: true}
}

// Truncate returns a copy of the Result with Content capped at maxBytes.
// If Content already fits, the original Result is returned unchanged.
// When truncation occurs the IsError flag is preserved and a footer is
// appended showing the original and new sizes.
func (r Result) Truncate(maxBytes int) Result {
	if len(r.Content) <= maxBytes {
		return r
	}
	suffix := fmt.Sprintf("\n\n[... truncated from %d bytes to %d bytes ...]", len(r.Content), maxBytes)
	cut := maxBytes - len(suffix)
	if cut < 0 {
		cut = 0
	}
	// Slice by runes so we never split a multi-byte UTF-8 codepoint.
	runes := []rune(r.Content)
	var n int
	for i, ru := range runes {
		next := n + len(string(ru))
		if next > cut {
			return Result{
				Content: string(runes[:i]) + suffix,
				IsError: r.IsError,
			}
		}
		n = next
	}
	return Result{
		Content: r.Content + suffix,
		IsError: r.IsError,
	}
}

// sprintf is a tiny indirection so callers don't pull in fmt
// transitively when they only want Errf.
func sprintf(format string, args ...any) string {
	if len(args) == 0 {
		return format
	}
	return fmtSprintf(format, args)
}
