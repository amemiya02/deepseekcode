package agent

import "testing"

// With a 1M usable window, an 800k-token session (0.80 raw) must NOT trigger
// compaction under the retuned, window-aware threshold.
func TestCompactionDefersWithLargeWindow(t *testing.T) {
	cfg := defaultSemanticCompactionConfig()
	cfg.UsableWindowTokens = 1_000_000
	pressure := 800_000.0 / float64(cfg.UsableWindowTokens) // 0.80
	action := ShouldSemanticCompact(pressure, cfg)
	if action == "compact" || action == "protect" {
		t.Fatal("compaction should defer at 0.80 of a 1M window after retune")
	}
}
