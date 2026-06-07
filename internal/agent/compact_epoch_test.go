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
