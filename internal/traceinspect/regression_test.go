package traceinspect_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/traceinspect"
)

// fixtureDir resolves the tests/fixtures path relative to the repo root,
// regardless of which directory the test binary runs from.
func fixtureDir(t *testing.T) string {
	t.Helper()
	// __file__ is this test file; go up: traceinspect/ → internal/ → repo root.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	return filepath.Join(repoRoot, "tests", "fixtures")
}

// Thresholds — tighten these as the codebase matures.
const (
	// MaxAllowedEvictions is the maximum number of non-expected-miss evictions
	// the fixture is allowed to contain. Zero means the fixture must be clean.
	MaxAllowedEvictions = 0

	// MinCacheHitRate is the minimum acceptable cache-hit rate (0..1) for the
	// fixture. Below this the gate fails and the developer must investigate.
	// InspectFile computes CacheHitRate over warm turns only (turns 2+ of each
	// epoch), excluding the expected cold-start first turn. With the 5-turn
	// fixture (turns 2-5 hit=173 000, miss=7 000) the warm rate is ~0.961,
	// comfortably above 0.90.
	MinCacheHitRate = 0.90
)

func TestCacheRegression_NoEvictions(t *testing.T) {
	fixture := filepath.Join(fixtureDir(t), "cache_regression.jsonl")
	ledger, err := traceinspect.ExplainFile(fixture)
	if err != nil {
		t.Fatalf("ExplainFile: %v", err)
	}

	var evictions int
	for _, row := range ledger {
		if row.Evicted {
			evictions++
			t.Logf("eviction at turn %d: why=%q hit=%d miss=%d cost=¥%.6f",
				row.Turn, row.Why, row.HitTokens, row.MissTokens, row.CostCNY)
		}
	}
	if evictions > MaxAllowedEvictions {
		t.Errorf("eviction regression: got %d evictions, max allowed %d", evictions, MaxAllowedEvictions)
	}
}

func TestCacheRegression_HitRate(t *testing.T) {
	fixture := filepath.Join(fixtureDir(t), "cache_regression.jsonl")
	rep, err := traceinspect.InspectFile(fixture)
	if err != nil {
		t.Fatalf("InspectFile: %v", err)
	}
	if rep.CacheHitRate < MinCacheHitRate {
		t.Errorf("cache-hit-rate regression: got %.3f, minimum %.3f (hit=%d miss=%d)",
			rep.CacheHitRate, MinCacheHitRate, rep.CacheHitTokens, rep.CacheMissTokens)
	}
}

func TestCacheRegression_TurnCount(t *testing.T) {
	fixture := filepath.Join(fixtureDir(t), "cache_regression.jsonl")
	ledger, err := traceinspect.ExplainFile(fixture)
	if err != nil {
		t.Fatalf("ExplainFile: %v", err)
	}
	// The fixture has 5 usage turns; if this changes, the fixture was edited.
	if len(ledger) != 5 {
		t.Errorf("fixture turn count changed: got %d, want 5 (was fixture edited?)", len(ledger))
	}
}
