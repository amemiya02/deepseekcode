package repair

import (
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

func TestScavengeToolCalls_DSMLShape(t *testing.T) {
	content := `{"name":"read_file","arguments":{"path":"README.md"}}`
	allowed := map[string]struct{}{"read_file": {}}

	result := ScavengeToolCalls("", content, allowed, ScavengeOptions{})

	if len(result.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(result.Calls))
	}
	if result.Calls[0].Function.Name != "read_file" {
		t.Errorf("expected name read_file, got %s", result.Calls[0].Function.Name)
	}
	if result.Calls[0].ID == "" {
		t.Error("expected non-empty ID")
	}
	if result.Calls[0].ID[:10] != "recovered_" {
		t.Errorf("expected ID prefix 'recovered_', got %s", result.Calls[0].ID[:10])
	}
}

func TestScavengeToolCalls_OpenAIShape(t *testing.T) {
	content := `{"type":"function","function":{"name":"read_file","arguments":"{\"path\":\"README.md\"}"}}`
	allowed := map[string]struct{}{"read_file": {}}

	result := ScavengeToolCalls("", content, allowed, ScavengeOptions{})

	if len(result.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(result.Calls))
	}
	if result.Calls[0].Function.Name != "read_file" {
		t.Errorf("expected name read_file, got %s", result.Calls[0].Function.Name)
	}
}

func TestScavengeToolCalls_UnknownTool(t *testing.T) {
	content := `{"name":"unknown_tool","arguments":{"path":"README.md"}}`
	allowed := map[string]struct{}{"read_file": {}}

	result := ScavengeToolCalls("", content, allowed, ScavengeOptions{})

	if len(result.Calls) != 0 {
		t.Errorf("expected 0 calls for unknown tool, got %d", len(result.Calls))
	}
}

func TestScavengeToolCalls_NaturalLanguage(t *testing.T) {
	content := `I should read README.md now`
	allowed := map[string]struct{}{"read_file": {}}

	result := ScavengeToolCalls("", content, allowed, ScavengeOptions{})

	if len(result.Calls) != 0 {
		t.Errorf("expected 0 calls for natural language, got %d", len(result.Calls))
	}
}

func TestScavengeToolCalls_Duplicate(t *testing.T) {
	content := `{"name":"read_file","arguments":{"path":"README.md"}} {"name":"read_file","arguments":{"path":"README.md"}}`
	allowed := map[string]struct{}{"read_file": {}}

	result := ScavengeToolCalls("", content, allowed, ScavengeOptions{})

	if len(result.Calls) != 1 {
		t.Errorf("expected 1 call after dedup, got %d", len(result.Calls))
	}
}

func TestScavengeToolCalls_ReasoningAndContent(t *testing.T) {
	reasoning := `{"name":"read_file","arguments":{"path":"a.txt"}}`
	content := `{"name":"read_file","arguments":{"path":"b.txt"}}`
	allowed := map[string]struct{}{"read_file": {}}

	result := ScavengeToolCalls(reasoning, content, allowed, ScavengeOptions{})

	if len(result.Calls) != 2 {
		t.Errorf("expected 2 calls from different sources, got %d", len(result.Calls))
	}
}

func TestScavengeToolCalls_DuplicateAcrossSources(t *testing.T) {
	reasoning := `{"name":"read_file","arguments":{"path":"README.md"}}`
	content := `{"name":"read_file","arguments":{"path":"README.md"}}`
	allowed := map[string]struct{}{"read_file": {}}

	result := ScavengeToolCalls(reasoning, content, allowed, ScavengeOptions{})

	if len(result.Calls) != 1 {
		t.Errorf("expected 1 call after dedup across sources, got %d", len(result.Calls))
	}
}

func TestScavengeToolCalls_MaxBytes(t *testing.T) {
	// Create content with tool call beyond MaxBytes
	prefix := "x"
	for i := 0; i < 100; i++ {
		prefix += "x"
	}
	content := prefix + `{"name":"read_file","arguments":{"path":"README.md"}}`
	allowed := map[string]struct{}{"read_file": {}}

	result := ScavengeToolCalls("", content, allowed, ScavengeOptions{MaxBytes: 50})

	if len(result.Calls) != 0 {
		t.Errorf("expected 0 calls when tool call is beyond MaxBytes, got %d", len(result.Calls))
	}
}

func TestScavengeToolCalls_MaxCalls(t *testing.T) {
	content := `{"name":"read_file","arguments":{"path":"a.txt"}} {"name":"read_file","arguments":{"path":"b.txt"}} {"name":"read_file","arguments":{"path":"c.txt"}}`
	allowed := map[string]struct{}{"read_file": {}}

	result := ScavengeToolCalls("", content, allowed, ScavengeOptions{MaxCalls: 2})

	if len(result.Calls) != 2 {
		t.Errorf("expected 2 calls with MaxCalls=2, got %d", len(result.Calls))
	}
}

func TestScavengeToolCalls_Defaults(t *testing.T) {
	content := `{"name":"read_file","arguments":{"path":"README.md"}}`
	allowed := map[string]struct{}{"read_file": {}}

	// Zero opts should apply defaults
	result := ScavengeToolCalls("", content, allowed, ScavengeOptions{})

	if len(result.Calls) != 1 {
		t.Errorf("expected 1 call with default opts, got %d", len(result.Calls))
	}
}

func TestScavengeToolCalls_EmptyAllowed(t *testing.T) {
	content := `{"name":"read_file","arguments":{"path":"README.md"}}`
	allowed := map[string]struct{}{}

	result := ScavengeToolCalls("", content, allowed, ScavengeOptions{})

	if len(result.Calls) != 0 {
		t.Errorf("expected 0 calls with empty allowed, got %d", len(result.Calls))
	}
}

func TestScavengeToolCalls_RepairArgs(t *testing.T) {
	// Malformed arguments that can be repaired
	content := `{"name":"read_file","arguments":{"path":"README.md"}`
	allowed := map[string]struct{}{"read_file": {}}

	result := ScavengeToolCalls("", content, allowed, ScavengeOptions{})

	if len(result.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(result.Calls))
	}
	// Arguments should be repaired
	args := result.Calls[0].Function.Arguments
	if args != `{"path":"README.md"}` {
		t.Errorf("expected repaired args, got %q", args)
	}
}

func TestScavengeToolCalls_InvalidArgsStillReturned(t *testing.T) {
	// Invalid arguments that cannot be repaired
	content := `{"name":"read_file","arguments":{this is not json}}`
	allowed := map[string]struct{}{"read_file": {}}

	result := ScavengeToolCalls("", content, allowed, ScavengeOptions{})

	// Should still return the call with invalid args
	if len(result.Calls) != 1 {
		t.Errorf("expected 1 call even with invalid args, got %d", len(result.Calls))
	}
	// Should have a report about invalid args
	foundInvalid := false
	for _, r := range result.Reports {
		if r.Kind == KindArgsInvalid {
			foundInvalid = true
		}
	}
	if !foundInvalid {
		t.Error("expected KindArgsInvalid report")
	}
}

func TestScavengeToolCalls_MultipleTools(t *testing.T) {
	content := `{"name":"read_file","arguments":{"path":"a.txt"}} {"name":"bash","arguments":{"cmd":"ls"}}`
	allowed := map[string]struct{}{"read_file": {}, "bash": {}}

	result := ScavengeToolCalls("", content, allowed, ScavengeOptions{})

	if len(result.Calls) != 2 {
		t.Errorf("expected 2 calls for different tools, got %d", len(result.Calls))
	}
}

func TestScavengeToolCalls_NestedJSON(t *testing.T) {
	// Tool call with nested JSON structure
	content := `{"name":"write_file","arguments":{"path":"test.txt","content":"hello\nworld"}}`
	allowed := map[string]struct{}{"write_file": {}}

	result := ScavengeToolCalls("", content, allowed, ScavengeOptions{})

	if len(result.Calls) != 1 {
		t.Errorf("expected 1 call with nested JSON, got %d", len(result.Calls))
	}
}

func TestScavengeToolCalls_KeyOrderDedup(t *testing.T) {
	// Same args with different key order should dedupe
	call1 := `{"name":"read_file","arguments":{"path":"README.md","limit":10}}`
	call2 := `{"name":"read_file","arguments":{"limit":10,"path":"README.md"}}`
	content := call1 + " " + call2
	allowed := map[string]struct{}{"read_file": {}}

	result := ScavengeToolCalls("", content, allowed, ScavengeOptions{})

	if len(result.Calls) != 1 {
		t.Errorf("expected 1 call after dedup by canonical args, got %d", len(result.Calls))
	}
}

func TestScavengeToolCalls_ReportKind(t *testing.T) {
	content := `{"name":"read_file","arguments":{"path":"README.md"}}`
	allowed := map[string]struct{}{"read_file": {}}

	result := ScavengeToolCalls("", content, allowed, ScavengeOptions{})

	if len(result.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(result.Calls))
	}
	// Verify call is properly formed
	if result.Calls[0].Type != "function" {
		t.Errorf("expected type 'function', got %q", result.Calls[0].Type)
	}
}

// Verify the call implements llm.ToolCall correctly
func TestScavengeToolCalls_ToolCallFields(t *testing.T) {
	content := `{"name":"read_file","arguments":{"path":"README.md"}}`
	allowed := map[string]struct{}{"read_file": {}}

	result := ScavengeToolCalls("", content, allowed, ScavengeOptions{})

	if len(result.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(result.Calls))
	}

	call := result.Calls[0]
	var _ llm.ToolCall = call // compile-time type check

	if call.ID == "" {
		t.Error("ID should not be empty")
	}
	if call.Function.Name == "" {
		t.Error("Function.Name should not be empty")
	}
	if call.Function.Arguments == "" {
		t.Error("Function.Arguments should not be empty")
	}
}

// TestParityScenario_tool_call_in_reasoning_scavenge pins that the
// repair system scavenges a valid tool call from reasoning text when
// the content field is empty or contains no tool calls.
func TestParityScenario_tool_call_in_reasoning_scavenge(t *testing.T) {
	reasoning := `I need to read the file. {"name":"read_file","arguments":{"path":"main.go"}}`
	allowed := map[string]struct{}{"read_file": {}}

	result := ScavengeToolCalls(reasoning, "", allowed, ScavengeOptions{})

	if len(result.Calls) != 1 {
		t.Fatalf("expected 1 call scavenged from reasoning, got %d", len(result.Calls))
	}
	if result.Calls[0].Function.Name != "read_file" {
		t.Errorf("expected read_file, got %s", result.Calls[0].Function.Name)
	}
}
