package repair

import (
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

func TestStormBreaker_Defaults(t *testing.T) {
	sb := NewStormBreaker(0, 0)
	if sb.window != 6 {
		t.Errorf("expected default window 6, got %d", sb.window)
	}
	if sb.threshold != 3 {
		t.Errorf("expected default threshold 3, got %d", sb.threshold)
	}
}

func TestStormBreaker_ThirdCallSuppressed(t *testing.T) {
	sb := NewStormBreaker(6, 3)
	kinds := map[string]ToolKind{"read_file": ToolReadOnly}

	calls := []llm.ToolCall{
		{ID: "1", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a"}`}},
		{ID: "2", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a"}`}},
		{ID: "3", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a"}`}},
	}

	result := sb.Filter(calls, kinds)

	if len(result.Calls) != 2 {
		t.Errorf("expected 2 kept calls, got %d", len(result.Calls))
	}
	if len(result.Reports) != 1 {
		t.Errorf("expected 1 suppression report, got %d", len(result.Reports))
	}
	if result.Reports[0].Kind != KindSuppressed {
		t.Errorf("expected KindSuppressed, got %s", result.Reports[0].Kind)
	}
}

func TestStormBreaker_MutatingNeverSuppressed(t *testing.T) {
	sb := NewStormBreaker(6, 3)
	kinds := map[string]ToolKind{"write_file": ToolMutating}

	// Same mutating call repeated many times
	calls := []llm.ToolCall{
		{ID: "1", Function: llm.ToolCallFunc{Name: "write_file", Arguments: `{"path":"a"}`}},
		{ID: "2", Function: llm.ToolCallFunc{Name: "write_file", Arguments: `{"path":"a"}`}},
		{ID: "3", Function: llm.ToolCallFunc{Name: "write_file", Arguments: `{"path":"a"}`}},
		{ID: "4", Function: llm.ToolCallFunc{Name: "write_file", Arguments: `{"path":"a"}`}},
	}

	result := sb.Filter(calls, kinds)

	if len(result.Calls) != 4 {
		t.Errorf("expected all 4 mutating calls kept, got %d", len(result.Calls))
	}
	if len(result.Reports) != 0 {
		t.Errorf("expected 0 reports for mutating calls, got %d", len(result.Reports))
	}
}

func TestStormBreaker_MutatingClearsHistory(t *testing.T) {
	sb := NewStormBreaker(6, 3)
	kinds := map[string]ToolKind{
		"read_file":  ToolReadOnly,
		"write_file": ToolMutating,
	}

	// Two read-only calls, then a mutating call, then same read-only again
	calls := []llm.ToolCall{
		{ID: "1", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a"}`}},
		{ID: "2", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a"}`}},
		{ID: "3", Function: llm.ToolCallFunc{Name: "write_file", Arguments: `{"path":"a"}`}},
		{ID: "4", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a"}`}},
	}

	result := sb.Filter(calls, kinds)

	// All 4 should be kept (history cleared after mutating call)
	if len(result.Calls) != 4 {
		t.Errorf("expected 4 kept calls after mutating clears history, got %d", len(result.Calls))
	}
}

func TestStormBreaker_Reset(t *testing.T) {
	sb := NewStormBreaker(6, 3)
	kinds := map[string]ToolKind{"read_file": ToolReadOnly}

	// Two calls first
	calls1 := []llm.ToolCall{
		{ID: "1", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a"}`}},
		{ID: "2", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a"}`}},
	}
	sb.Filter(calls1, kinds)

	// Reset
	sb.Reset()

	// Same calls again - should start fresh
	calls2 := []llm.ToolCall{
		{ID: "3", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a"}`}},
		{ID: "4", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a"}`}},
		{ID: "5", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a"}`}},
	}

	result := sb.Filter(calls2, kinds)

	// After reset, only third call is suppressed
	if len(result.Calls) != 2 {
		t.Errorf("expected 2 kept calls after reset, got %d", len(result.Calls))
	}
}

func TestStormBreaker_DifferentArgsNotSuppressed(t *testing.T) {
	sb := NewStormBreaker(6, 3)
	kinds := map[string]ToolKind{"read_file": ToolReadOnly}

	calls := []llm.ToolCall{
		{ID: "1", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a"}`}},
		{ID: "2", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"b"}`}},
		{ID: "3", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"c"}`}},
	}

	result := sb.Filter(calls, kinds)

	if len(result.Calls) != 3 {
		t.Errorf("expected all 3 calls kept (different args), got %d", len(result.Calls))
	}
}

func TestStormBreaker_KeyOrderDedup(t *testing.T) {
	sb := NewStormBreaker(6, 3)
	kinds := map[string]ToolKind{"read_file": ToolReadOnly}

	// Same args with different key order should count as same
	calls := []llm.ToolCall{
		{ID: "1", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a","limit":10}`}},
		{ID: "2", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"limit":10,"path":"a"}`}},
		{ID: "3", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a","limit":10}`}},
	}

	result := sb.Filter(calls, kinds)

	if len(result.Calls) != 2 {
		t.Errorf("expected 2 kept calls (key order dedup), got %d", len(result.Calls))
	}
}

func TestStormBreaker_InvalidJSONArgs(t *testing.T) {
	sb := NewStormBreaker(6, 3)
	kinds := map[string]ToolKind{"read_file": ToolReadOnly}

	// Invalid JSON args should use raw args for identity
	calls := []llm.ToolCall{
		{ID: "1", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{invalid`}},
		{ID: "2", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{invalid`}},
		{ID: "3", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{invalid`}},
	}

	result := sb.Filter(calls, kinds)

	if len(result.Calls) != 2 {
		t.Errorf("expected 2 kept calls (invalid JSON dedup by raw), got %d", len(result.Calls))
	}
}

func TestStormBreaker_WindowSize(t *testing.T) {
	sb := NewStormBreaker(3, 3) // Small window
	kinds := map[string]ToolKind{"read_file": ToolReadOnly}

	// 3 calls, but window is 3, so third should be suppressed
	calls := []llm.ToolCall{
		{ID: "1", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a"}`}},
		{ID: "2", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a"}`}},
		{ID: "3", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a"}`}},
	}

	result := sb.Filter(calls, kinds)

	if len(result.Calls) != 2 {
		t.Errorf("expected 2 kept calls with window=3, got %d", len(result.Calls))
	}
}

func TestStormBreaker_UnknownToolIsMutating(t *testing.T) {
	sb := NewStormBreaker(6, 3)
	kinds := map[string]ToolKind{} // Empty kinds - unknown tools

	calls := []llm.ToolCall{
		{ID: "1", Function: llm.ToolCallFunc{Name: "unknown", Arguments: `{}`}},
		{ID: "2", Function: llm.ToolCallFunc{Name: "unknown", Arguments: `{}`}},
		{ID: "3", Function: llm.ToolCallFunc{Name: "unknown", Arguments: `{}`}},
	}

	result := sb.Filter(calls, kinds)

	// Unknown tools are treated as mutating, so all kept
	if len(result.Calls) != 3 {
		t.Errorf("expected all 3 unknown tools kept (treated as mutating), got %d", len(result.Calls))
	}
}

func TestStormBreaker_MixedCalls(t *testing.T) {
	sb := NewStormBreaker(6, 3)
	kinds := map[string]ToolKind{
		"read_file":  ToolReadOnly,
		"write_file": ToolMutating,
	}

	calls := []llm.ToolCall{
		{ID: "1", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a"}`}},
		{ID: "2", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a"}`}},
		{ID: "3", Function: llm.ToolCallFunc{Name: "write_file", Arguments: `{"path":"b"}`}},
		{ID: "4", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a"}`}},
		{ID: "5", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a"}`}},
		{ID: "6", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a"}`}},
	}

	result := sb.Filter(calls, kinds)

	// After write_file clears history, read_file starts fresh
	// So calls 4,5,6 are counted as 1,2,3 -> 6th suppressed
	if len(result.Calls) != 5 {
		t.Errorf("expected 5 kept calls in mixed scenario, got %d", len(result.Calls))
	}
	if len(result.Reports) != 1 {
		t.Errorf("expected 1 suppression report, got %d", len(result.Reports))
	}
}

func TestStormBreaker_ReportFields(t *testing.T) {
	sb := NewStormBreaker(6, 3)
	kinds := map[string]ToolKind{"read_file": ToolReadOnly}

	calls := []llm.ToolCall{
		{ID: "call_1", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a"}`}},
		{ID: "call_2", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a"}`}},
		{ID: "call_3", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a"}`}},
	}

	result := sb.Filter(calls, kinds)

	if len(result.Reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(result.Reports))
	}

	r := result.Reports[0]
	if r.Tool != "read_file" {
		t.Errorf("expected Tool='read_file', got %q", r.Tool)
	}
	if r.CallID != "call_3" {
		t.Errorf("expected CallID='call_3', got %q", r.CallID)
	}
	if r.Message == "" {
		t.Error("expected non-empty Message")
	}
}

func TestStormBreaker_EmptyInput(t *testing.T) {
	sb := NewStormBreaker(6, 3)
	kinds := map[string]ToolKind{"read_file": ToolReadOnly}

	result := sb.Filter([]llm.ToolCall{}, kinds)

	if len(result.Calls) != 0 {
		t.Errorf("expected 0 calls for empty input, got %d", len(result.Calls))
	}
	if len(result.Reports) != 0 {
		t.Errorf("expected 0 reports for empty input, got %d", len(result.Reports))
	}
}
