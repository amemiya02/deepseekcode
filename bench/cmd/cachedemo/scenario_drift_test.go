// bench/cmd/cachedemo/scenario_drift_test.go
package main

import (
	"encoding/json"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// TestDriftPrefixByteEqualsStableBeforeDrift verifies that driftPrefix for
// turns before the drift point is byte-identical to stablePrefix (same
// serialized bytes, same fingerprint) — the prefix has not yet drifted.
func TestDriftPrefixByteEqualsStableBeforeDrift(t *testing.T) {
	stable := stablePrefix()
	drift := driftPrefix(1, 3)

	// Compare via llm.Request marshaling: same bytes on the wire means same
	// cache key on DeepSeek's side.
	stableBytes, err := marshalPrefix(stable)
	if err != nil {
		t.Fatalf("marshal stable: %v", err)
	}
	driftBytes, err := marshalPrefix(drift)
	if err != nil {
		t.Fatalf("marshal drift: %v", err)
	}
	if string(stableBytes) != string(driftBytes) {
		t.Errorf("driftPrefix(turn=1, driftAt=3) should byte-equal stablePrefix before drift point\nstable: %s\ndrift:  %s", stableBytes, driftBytes)
	}

	// Also compare fingerprints for consistency.
	if stable.Fingerprint() != drift.Fingerprint() {
		t.Errorf("fingerprint mismatch before drift: stable=%v drift=%v", stable.Fingerprint(), drift.Fingerprint())
	}
}

// TestDriftPrefixBytesDifferAtDriftPoint verifies that driftPrefix at the drift
// turn (turn == driftAt) has different bytes from stablePrefix — the extra
// "notify" tool changes the serialized form, busting the cache.
func TestDriftPrefixBytesDifferAtDriftPoint(t *testing.T) {
	stable := stablePrefix()
	drift := driftPrefix(3, 3)

	stableBytes, err := marshalPrefix(stable)
	if err != nil {
		t.Fatalf("marshal stable: %v", err)
	}
	driftBytes, err := marshalPrefix(drift)
	if err != nil {
		t.Fatalf("marshal drift: %v", err)
	}
	if string(stableBytes) == string(driftBytes) {
		t.Error("driftPrefix(turn=3, driftAt=3) should NOT byte-equal stablePrefix at drift point (extra tool must change bytes)")
	}

	// Fingerprints must also differ.
	if stable.Fingerprint() == drift.Fingerprint() {
		t.Error("fingerprint should differ at drift point: extra tool was not reflected")
	}

	// The drift prefix should carry exactly one more tool than stable.
	if len(drift.Tools) != len(stable.Tools)+1 {
		t.Errorf("drift prefix should have %d tools, got %d", len(stable.Tools)+1, len(drift.Tools))
	}
}

// marshalPrefix uses llm marshaling (the canonical wire form) to serialize a
// StaticPrefix so the test compares the exact bytes DeepSeek would see.
func marshalPrefix(p llm.StaticPrefix) ([]byte, error) {
	return json.Marshal(p.Tools)
}
