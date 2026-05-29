package repair

import (
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// roCall builds a read-only tool call for the storm-breaker tests.
func roCall(id, args string) llm.ToolCall {
	return llm.ToolCall{ID: id, Function: llm.ToolCallFunc{Name: "read_file", Arguments: args}}
}

// TestStormSnapshotRestore pins that Snapshot/Restore rewinds suppression
// history so a throwaway batch does not affect later filtering — the primitive
// the agent's escalation path relies on to discard a flash turn cleanly.
func TestStormSnapshotRestore(t *testing.T) {
	kinds := map[string]ToolKind{"read_file": ToolReadOnly}
	sb := NewStormBreaker(6, 3)

	snap := sb.Snapshot() // empty baseline

	// Run a "throwaway" batch of two identical read-only calls (both kept,
	// history now holds two occurrences).
	r := sb.Filter([]llm.ToolCall{roCall("1", `{"p":"a"}`), roCall("2", `{"p":"a"}`)}, kinds)
	if len(r.Calls) != 2 {
		t.Fatalf("throwaway batch: kept %d, want 2", len(r.Calls))
	}

	// Without restore, a third identical call would be suppressed (count 2 >=
	// threshold-1). Confirm that, then rewind.
	if got := sb.Filter([]llm.ToolCall{roCall("3", `{"p":"a"}`)}, kinds); len(got.Calls) != 0 || len(got.Reports) != 1 {
		t.Fatalf("pre-restore: expected the 3rd identical call suppressed, kept=%d reports=%d", len(got.Calls), len(got.Reports))
	}

	sb.Restore(snap)

	// After restore the history is empty again, so an identical call is kept.
	if got := sb.Filter([]llm.ToolCall{roCall("4", `{"p":"a"}`)}, kinds); len(got.Calls) != 1 || len(got.Reports) != 0 {
		t.Fatalf("post-restore: expected the call kept (clean history), kept=%d reports=%d", len(got.Calls), len(got.Reports))
	}
}

// TestStormSnapshotIsIndependentCopy pins that mutating the breaker after a
// snapshot does not change the snapshot (it is a copy, not an alias).
func TestStormSnapshotIsIndependentCopy(t *testing.T) {
	kinds := map[string]ToolKind{"read_file": ToolReadOnly}
	sb := NewStormBreaker(6, 3)
	sb.Filter([]llm.ToolCall{roCall("1", `{"p":"a"}`)}, kinds) // history = [a]
	snap := sb.Snapshot()

	// Mutate further, then restore to the snapshot — only the [a] state returns.
	sb.Filter([]llm.ToolCall{roCall("2", `{"p":"a"}`)}, kinds) // history = [a, a]
	sb.Restore(snap)

	// History is back to a single [a]: one more identical call is kept (count 1
	// < threshold-1), a second would be suppressed.
	if got := sb.Filter([]llm.ToolCall{roCall("3", `{"p":"a"}`)}, kinds); len(got.Calls) != 1 {
		t.Fatalf("expected restore to [a]; kept=%d (want 1)", len(got.Calls))
	}
}
