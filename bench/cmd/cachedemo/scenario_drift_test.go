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
	drift := driftPrefix(1, 3, "n")

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
		t.Errorf("driftPrefix(turn=1, driftAt=3, nonce) should byte-equal stablePrefix before drift point\nstable: %s\ndrift:  %s", stableBytes, driftBytes)
	}

	// Also compare fingerprints for consistency.
	if stable.Fingerprint() != drift.Fingerprint() {
		t.Errorf("fingerprint mismatch before drift: stable=%v drift=%v", stable.Fingerprint(), drift.Fingerprint())
	}
}

// TestDriftPrefixBytesDifferAtDriftPoint verifies that driftPrefix at the drift
// turn (turn == driftAt) has different bytes from stablePrefix — the extra
// "notify" tool (carrying the nonce) changes the serialized form, busting the cache.
func TestDriftPrefixBytesDifferAtDriftPoint(t *testing.T) {
	stable := stablePrefix()
	drift := driftPrefix(3, 3, "nonceA")

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

// TestDriftPrefixPostDriftDiffersAcrossNonces verifies that post-drift prefixes
// from two different runs (nonces) differ, ensuring each run is a genuine cache
// miss rather than a cross-run warmed hit.
func TestDriftPrefixPostDriftDiffersAcrossNonces(t *testing.T) {
	driftA := driftPrefix(3, 3, "nonceA")
	driftB := driftPrefix(3, 3, "nonceB")
	if driftA.Fingerprint() == driftB.Fingerprint() {
		t.Fatal("driftPrefix(3,3,nonceA) and driftPrefix(3,3,nonceB) must differ (nonce isolates runs)")
	}
}

// TestDriftPrefixPreDriftByteEqualsStableRegardlessOfNonce verifies that the
// nonce does NOT affect pre-drift turns — they remain byte-identical to
// stablePrefix() so the demonstrable cache hit still works.
func TestDriftPrefixPreDriftByteEqualsStableRegardlessOfNonce(t *testing.T) {
	stable := stablePrefix()
	driftA := driftPrefix(1, 3, "nonceA")
	driftB := driftPrefix(1, 3, "nonceB")

	if stable.Fingerprint() != driftA.Fingerprint() {
		t.Fatal("pre-drift driftPrefix(nonceA) must equal stablePrefix()")
	}
	if stable.Fingerprint() != driftB.Fingerprint() {
		t.Fatal("pre-drift driftPrefix(nonceB) must equal stablePrefix()")
	}
}

// marshalPrefix uses llm marshaling (the canonical wire form) to serialize a
// StaticPrefix so the test compares the exact bytes DeepSeek would see.
func marshalPrefix(p llm.StaticPrefix) ([]byte, error) {
	return json.Marshal(p.Tools)
}
