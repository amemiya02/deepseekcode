package agent

import (
	"testing"

	"github.com/amemiya02/deepseekcode/internal/cache"
	"github.com/amemiya02/deepseekcode/internal/llm"
)

func TestEpochBumpsOnlyOnCompaction(t *testing.T) {
	var e cacheEpochCounter
	start := e.value()
	e.afterTurnNoCompaction()
	if e.value() != start {
		t.Fatalf("epoch changed without compaction: %d -> %d", start, e.value())
	}
	e.afterCompaction()
	if e.value() != start+1 {
		t.Fatalf("epoch did not bump on compaction: %d -> %d", start, e.value())
	}
	// Second compaction bumps again.
	e.afterCompaction()
	if e.value() != start+2 {
		t.Fatalf("epoch did not bump on second compaction: %d -> %d", start, e.value())
	}
	// No-compaction turn is still a no-op.
	e.afterTurnNoCompaction()
	if e.value() != start+2 {
		t.Fatalf("epoch changed on non-compaction turn: %d -> %d", start+2, e.value())
	}
}

func TestCompactionCountIsObservable(t *testing.T) {
	var m compactionMetrics
	if m.CompactionCount() != 0 {
		t.Fatalf("initial compaction count = %d, want 0", m.CompactionCount())
	}
	m.record(0.005) // re-prefill cost estimate (CNY)
	if m.CompactionCount() != 1 {
		t.Fatalf("count after one compaction = %d, want 1", m.CompactionCount())
	}
	m.record(0.010)
	if m.CompactionCount() != 2 {
		t.Fatalf("count after two compactions = %d, want 2", m.CompactionCount())
	}
	if m.CompactionLastCost() != 0.010 {
		t.Fatalf("last cost = %f, want 0.010", m.CompactionLastCost())
	}
}

// TestCompactionEpochAttributionEndToEnd verifies that after a compaction
// bumps the epoch, the NEXT turn's cache receipt is attributed as
// compact_reset. This is the end-to-end guarantee: the re-prefill cost
// is paid on the request that follows compaction, not the turn that
// triggered it (maybeCompact runs at the END of each loop iteration,
// so the bumped epoch is visible to the next turn's runStep).
func TestCompactionEpochAttributionEndToEnd(t *testing.T) {
	var epoch cacheEpochCounter
	prevEpoch := cache.Epoch(epoch.value())

	// Turn 1: no compaction — receipt should be steady (prefix unchanged).
	curEpoch := cache.Epoch(epoch.value())
	base := llm.StaticPrefix{System: "SYS"}
	receipt1 := cache.Attribute(cache.Input{
		Turn: 1, Model: "deepseek-v4-flash",
		Prev: &base, Cur: base,
		PrevEpoch: prevEpoch, CurEpoch: curEpoch,
		Usage: llm.Usage{PromptCacheHitTokens: 1000, PromptCacheMissTokens: 100},
	})
	if receipt1.Dominant != cache.CauseSteady {
		t.Fatalf("turn 1 Dominant = %q, want steady", receipt1.Dominant)
	}

	// Compaction happens at the end of turn 1 (between loop iterations).
	epoch.afterCompaction()
	prevEpoch = curEpoch

	// Turn 2: epoch bumped — receipt MUST be compact_reset.
	curEpoch = cache.Epoch(epoch.value())
	receipt2 := cache.Attribute(cache.Input{
		Turn: 2, Model: "deepseek-v4-flash",
		Prev: &base, Cur: base,
		PrevEpoch: prevEpoch, CurEpoch: curEpoch,
		Usage: llm.Usage{PromptCacheHitTokens: 0, PromptCacheMissTokens: 5000},
	})
	if receipt2.Dominant != cache.CauseCompactReset {
		t.Fatalf("turn 2 Dominant = %q, want compact_reset (epoch bumped)", receipt2.Dominant)
	}

	// Turn 3: no further compaction — back to steady.
	prevEpoch = curEpoch
	curEpoch = cache.Epoch(epoch.value())
	receipt3 := cache.Attribute(cache.Input{
		Turn: 3, Model: "deepseek-v4-flash",
		Prev: &base, Cur: base,
		PrevEpoch: prevEpoch, CurEpoch: curEpoch,
		Usage: llm.Usage{PromptCacheHitTokens: 2000, PromptCacheMissTokens: 50},
	})
	if receipt3.Dominant != cache.CauseSteady {
		t.Fatalf("turn 3 Dominant = %q, want steady (no compaction)", receipt3.Dominant)
	}
}
