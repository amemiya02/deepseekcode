// bench/cmd/cachedemo/scenario_test.go
package main

import "testing"

func TestStablePrefixFingerprintConstantAcrossTurns(t *testing.T) {
	// The cache-stable arm must present byte-identical system+tools every turn,
	// so its Prefix Fingerprint (== DeepSeek cache key) never moves.
	fp1 := stablePrefix().Fingerprint()
	fp2 := stablePrefix().Fingerprint()
	if fp1 != fp2 {
		t.Fatal("stable prefix fingerprint is not constant")
	}
	// Sanity-check: the stable fingerprint must differ from the naive prefix.
	if fp1 == naivePrefix(1, "nonce1").Fingerprint() {
		t.Fatal("stable prefix fingerprint must differ from naive prefix fingerprint")
	}
}

func TestNaivePrefixFingerprintChangesEachTurn(t *testing.T) {
	// The cache-naive arm mutates the prefix each turn (the generic-agent
	// failure mode), so consecutive turns must differ -> cache miss.
	if naivePrefix(1, "nonce1").Fingerprint() == naivePrefix(2, "nonce1").Fingerprint() {
		t.Fatal("naive prefix fingerprint did not change between turns")
	}
}

// TestNaivePrefixDiffersAcrossNonces verifies that the same turn with
// different nonces produces different bytes — each run is a genuine cache miss.
func TestNaivePrefixDiffersAcrossNonces(t *testing.T) {
	if naivePrefix(1, "nonceA").Fingerprint() == naivePrefix(1, "nonceB").Fingerprint() {
		t.Fatal("naivePrefix(1,nonceA) must differ from naivePrefix(1,nonceB)")
	}
}

// TestStablePrefixHasNoNonce verifies stablePrefix() is byte-identical across
// calls (no nonce injected) — cross-session warmth is a feature.
func TestStablePrefixHasNoNonce(t *testing.T) {
	fp1 := stablePrefix().Fingerprint()
	fp2 := stablePrefix().Fingerprint()
	if fp1 != fp2 {
		t.Fatal("stablePrefix must be byte-identical across calls (no nonce)")
	}
}

func TestDemoTurns(t *testing.T) {
	base := []string{
		"List the Go files in internal/llm.",
		"Read internal/llm/cache_metrics.go and summarize Cost().",
		"What does CacheSavings compute?",
		"Find where prompt_cache_hit_tokens is read.",
		"Explain the prefix fingerprint in one sentence.",
		"Which models are in the Prices table?",
	}
	cases := []struct {
		n    int
		want []string
	}{
		{n: 0, want: []string{}},
		{n: 6, want: base[:6]},
		{n: 7, want: append(base[:6:6], base[0])},
	}
	for _, tc := range cases {
		got := demoTurns(tc.n)
		if len(got) != len(tc.want) {
			t.Errorf("demoTurns(%d): got len %d, want %d", tc.n, len(got), len(tc.want))
			continue
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Errorf("demoTurns(%d)[%d]: got %q, want %q", tc.n, i, got[i], tc.want[i])
			}
		}
	}
}

func TestBuildRequestSetsStableHeadAndIncludesUsage(t *testing.T) {
	r := buildRequest("deepseek-v4-flash", stablePrefix(), "list the files")
	if r.StreamOptions == nil || !r.StreamOptions.IncludeUsage {
		t.Fatal("IncludeUsage must be set so the final SSE frame carries cache tokens")
	}
	if len(r.Tools) == 0 {
		t.Fatal("request must carry the tool prefix")
	}
}
