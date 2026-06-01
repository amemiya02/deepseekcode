package repair

import (
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

func TestHashArgs_Empty(t *testing.T) {
	h1 := HashArgs("")
	h2 := HashArgs("")
	if h1 != h2 {
		t.Errorf("empty string hash not consistent: %s vs %s", h1, h2)
	}
	// SHA-256 produces 64 hex characters
	if len(h1) != 64 {
		t.Errorf("expected 64-char hex hash, got %d", len(h1))
	}
}

func TestHashArgs_Consistent(t *testing.T) {
	input := `{"path":"README.md"}`
	h1 := HashArgs(input)
	h2 := HashArgs(input)
	if h1 != h2 {
		t.Errorf("same input produced different hashes: %s vs %s", h1, h2)
	}
}

func TestHashArgs_Different(t *testing.T) {
	h1 := HashArgs(`{"a":1}`)
	h2 := HashArgs(`{"a":2}`)
	if h1 == h2 {
		t.Errorf("different inputs produced same hash")
	}
}

func TestCanonicalArgs_Valid(t *testing.T) {
	input := `{"b":2,"a":1}`
	out, ok := CanonicalArgs(input)
	if !ok {
		t.Fatalf("expected valid JSON, got false")
	}
	expected := `{"a":1,"b":2}`
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}

func TestCanonicalArgs_Nested(t *testing.T) {
	input := `{"b":1,"a":{"d":4,"c":3}}`
	out, ok := CanonicalArgs(input)
	if !ok {
		t.Fatalf("expected valid JSON, got false")
	}
	expected := `{"a":{"c":3,"d":4},"b":1}`
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}

func TestCanonicalArgs_Invalid(t *testing.T) {
	input := `{`
	out, ok := CanonicalArgs(input)
	if ok {
		t.Errorf("expected false for invalid JSON, got true")
	}
	if out != input {
		t.Errorf("expected original input returned, got %q", out)
	}
}

func TestCanonicalArgs_ArrayPreservesOrder(t *testing.T) {
	input := `[3,2,1]`
	out, ok := CanonicalArgs(input)
	if !ok {
		t.Fatalf("expected valid JSON, got false")
	}
	// Arrays preserve order
	expected := `[3,2,1]`
	if out != expected {
		t.Errorf("expected array order preserved %q, got %q", expected, out)
	}
}

func TestCanonicalArgs_ArrayWithObjects(t *testing.T) {
	input := `[{"b":2},{"a":1}]`
	out, ok := CanonicalArgs(input)
	if !ok {
		t.Fatalf("expected valid JSON, got false")
	}
	expected := `[{"b":2},{"a":1}]`
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}

func TestCanonicalArgs_Empty(t *testing.T) {
	out, ok := CanonicalArgs("")
	if ok {
		t.Errorf("empty string is not valid JSON, expected false")
	}
	if out != "" {
		t.Errorf("expected original input returned, got %q", out)
	}
}

func TestCanonicalArgs_DeepNesting(t *testing.T) {
	input := `{"z":{"y":{"x":1}}}`
	out, ok := CanonicalArgs(input)
	if !ok {
		t.Fatalf("expected valid JSON, got false")
	}
	// Already sorted, should be unchanged
	expected := `{"z":{"y":{"x":1}}}`
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}

func TestCanonicalArgs_MixedTypes(t *testing.T) {
	input := `{"c":"str","b":123,"a":true,"d":null}`
	out, ok := CanonicalArgs(input)
	if !ok {
		t.Fatalf("expected valid JSON, got false")
	}
	// Keys should be sorted alphabetically
	expected := `{"a":true,"b":123,"c":"str","d":null}`
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}

func TestPipeline_Repair_NoOpReturnsSameCall(t *testing.T) {
	p := NewPipeline()
	original := llm.ToolCall{
		ID:   "call_123",
		Type: "function",
		Function: llm.ToolCallFunc{
			Name:      "read",
			Arguments: `{"path":"/src/file.go"}`,
		},
	}

	repaired, report := p.Repair(original)

	// Should return the exact same call byte-for-byte
	if repaired.ID != original.ID {
		t.Errorf("ID mismatch: got %q, want %q", repaired.ID, original.ID)
	}
	if repaired.Type != original.Type {
		t.Errorf("Type mismatch: got %q, want %q", repaired.Type, original.Type)
	}
	if repaired.Function.Name != original.Function.Name {
		t.Errorf("Function.Name mismatch: got %q, want %q", repaired.Function.Name, original.Function.Name)
	}
	if repaired.Function.Arguments != original.Function.Arguments {
		t.Errorf("Function.Arguments mismatch: got %q, want %q", repaired.Function.Arguments, original.Function.Arguments)
	}
	_ = report // No-op returns a report
}

func TestPipeline_Repair_NoOpReportsActionNone(t *testing.T) {
	p := NewPipeline()
	call := llm.ToolCall{
		ID:   "call_456",
		Type: "function",
		Function: llm.ToolCallFunc{
			Name:      "write",
			Arguments: `{"path":"/tmp/test.txt"}`,
		},
	}

	_, report := p.Repair(call)

	if report.Kind != KindNone {
		t.Errorf("expected Kind %q, got %q", KindNone, report.Kind)
	}
	if report.Tool != "write" {
		t.Errorf("expected Tool %q, got %q", "write", report.Tool)
	}
	if report.CallID != "call_456" {
		t.Errorf("expected CallID %q, got %q", "call_456", report.CallID)
	}
	if report.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestPipeline_Repair_UnknownToolNotRewritten(t *testing.T) {
	p := NewPipeline()
	original := llm.ToolCall{
		ID:   "call_789",
		Type: "function",
		Function: llm.ToolCallFunc{
			Name:      "unknown_tool_xyz",
			Arguments: `{"foo":"bar"}`,
		},
	}

	repaired, _ := p.Repair(original)

	// Unknown tools should pass through unchanged
	if repaired.Function.Name != "unknown_tool_xyz" {
		t.Errorf("unknown tool was rewritten to %q", repaired.Function.Name)
	}
	if repaired.Function.Arguments != `{"foo":"bar"}` {
		t.Errorf("arguments were changed to %q", repaired.Function.Arguments)
	}
}

func TestPipeline_Repair_EmptyToolNameNoPanic(t *testing.T) {
	p := NewPipeline()
	call := llm.ToolCall{
		ID:   "call_empty",
		Type: "function",
		Function: llm.ToolCallFunc{
			Name:      "",
			Arguments: `{}`,
		},
	}

	// Should not panic
	repaired, report := p.Repair(call)

	if repaired.Function.Name != "" {
		t.Errorf("empty tool name was changed to %q", repaired.Function.Name)
	}
	if report.Tool != "" {
		t.Errorf("report tool should be empty, got %q", report.Tool)
	}
}

func TestPipeline_Repair_EmptyArgumentsNoPanic(t *testing.T) {
	p := NewPipeline()
	call := llm.ToolCall{
		ID:   "call_noargs",
		Type: "function",
		Function: llm.ToolCallFunc{
			Name:      "bash",
			Arguments: "",
		},
	}

	// Should not panic
	repaired, report := p.Repair(call)

	if repaired.Function.Arguments != "" {
		t.Errorf("empty arguments were changed to %q", repaired.Function.Arguments)
	}
	if report.Tool != "bash" {
		t.Errorf("expected tool %q, got %q", "bash", report.Tool)
	}
}

func TestKindFromAction_Mapping(t *testing.T) {
	tests := []struct {
		action   Action
		expected Kind
	}{
		{ActionNone, KindNone},
		{ActionRepaired, KindArgsCompleted},
		{ActionRejected, KindArgsInvalid},
		{ActionContinue, KindContinue},
		{Action("unknown"), KindNone},
	}

	for _, tt := range tests {
		t.Run(string(tt.action), func(t *testing.T) {
			got := KindFromAction(tt.action)
			if got != tt.expected {
				t.Errorf("KindFromAction(%q) = %q, want %q", tt.action, got, tt.expected)
			}
		})
	}
}
