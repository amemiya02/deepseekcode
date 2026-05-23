package tools

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

// sprintf is a tiny indirection so callers don't pull in fmt
// transitively when they only want Errf.
func sprintf(format string, args ...any) string {
	if len(args) == 0 {
		return format
	}
	return fmtSprintf(format, args)
}
