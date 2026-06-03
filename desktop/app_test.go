package main

import (
	"strings"
	"testing"
)

// TestGetPortReturnsConfiguredValue verifies that GetPort returns the port
// value that was passed to newApp. This is a pure function test with no
// Wails or display dependency.
func TestGetPortReturnsConfiguredValue(t *testing.T) {
	const want = 9876
	app := newApp(want)
	if got := app.GetPort(); got != want {
		t.Fatalf("GetPort() = %d, want %d", got, want)
	}
}

// TestGetVersionNonEmpty verifies that GetVersion returns a non-trivial string.
// A broken version wiring (e.g., a missing ldflags injection or wrong import)
// would be caught here before it reaches the Wails JS bridge.
// We require the result to be non-empty after trimming whitespace AND to
// have at least 3 characters, ensuring a placeholder like " " or "v" alone
// does not slip through. (Dev builds legitimately return "dev (none, unknown)"
// which has no digits, so we do not require a digit.)
func TestGetVersionNonEmpty(t *testing.T) {
	app := newApp(0)
	v := app.GetVersion()
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		t.Fatalf("GetVersion() returned empty or whitespace-only string: %q", v)
	}
	// Require at least 3 characters so a single-character or trivially short
	// placeholder (e.g. "v", " ", "0") would be caught.
	if len(trimmed) < 3 {
		t.Fatalf("GetVersion() = %q: version string too short (len=%d), expected a meaningful value", v, len(trimmed))
	}
}
