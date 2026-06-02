// bench/cmd/cachedemo/scenario_test.go
package main

import "testing"

func TestStablePrefixFingerprintConstantAcrossTurns(t *testing.T) {
	// The cache-stable arm must present byte-identical system+tools every turn,
	// so its Prefix Fingerprint (== DeepSeek cache key) never moves.
	if stablePrefix().Fingerprint() != stablePrefix().Fingerprint() {
		t.Fatal("stable prefix fingerprint is not constant")
	}
}

func TestNaivePrefixFingerprintChangesEachTurn(t *testing.T) {
	// The cache-naive arm mutates the prefix each turn (the generic-agent
	// failure mode), so consecutive turns must differ -> cache miss.
	if naivePrefix(1).Fingerprint() == naivePrefix(2).Fingerprint() {
		t.Fatal("naive prefix fingerprint did not change between turns")
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
