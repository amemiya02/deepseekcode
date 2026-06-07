package agent

import "testing"

// fimFastPathConfig is a minimal config stub for testing.
type fimFastPathConfig struct {
	enabled bool
}

func (c fimFastPathConfig) FIMFastPathEnabled() bool { return c.enabled }

func TestFIMFastPathDisabledByDefault(t *testing.T) {
	cfg := fimFastPathConfig{enabled: false}
	if fimFastPathEnabled(cfg) {
		t.Fatal("FIM fast path must default OFF")
	}
}

func TestFIMFastPathEnabledWhenOptedIn(t *testing.T) {
	cfg := fimFastPathConfig{enabled: true}
	if !fimFastPathEnabled(cfg) {
		t.Fatal("FIM fast path should be ON when opted in")
	}
}

func TestFIMEligibilityIsConservative(t *testing.T) {
	// Large edit: not eligible
	if eligibleForFIM(editRequest{LinesChanged: 500, SingleHunk: true}) {
		t.Fatal("large edits must not be FIM-eligible")
	}
	// Multi-hunk: not eligible
	if eligibleForFIM(editRequest{LinesChanged: 3, SingleHunk: false}) {
		t.Fatal("multi-hunk edits must not be FIM-eligible")
	}
	// Small single-hunk: eligible
	if !eligibleForFIM(editRequest{LinesChanged: 3, SingleHunk: true}) {
		t.Fatal("small single-hunk edit should be FIM-eligible when enabled")
	}
	// At the boundary (20 lines): eligible
	if !eligibleForFIM(editRequest{LinesChanged: 20, SingleHunk: true}) {
		t.Fatal("20-line single-hunk edit should be FIM-eligible (boundary)")
	}
	// Over the boundary (21 lines): not eligible
	if eligibleForFIM(editRequest{LinesChanged: 21, SingleHunk: true}) {
		t.Fatal("21-line edit should not be FIM-eligible (over boundary)")
	}
}
