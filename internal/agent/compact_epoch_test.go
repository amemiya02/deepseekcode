package agent

import "testing"

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
