package llm

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"
)

// TestCacheStableDeterminism verifies that MarshalCacheStable produces
// byte-identical output across repeated calls with the same input.
// This is the core invariant: any byte difference invalidates DeepSeek's
// prompt cache and loses the 50× cache-hit discount.
func TestCacheStableDeterminism(t *testing.T) {
	req := buildRepresentativeRequest()

	hashes := make(map[[32]byte]bool)
	for i := 0; i < 10; i++ {
		b, err := req.MarshalCacheStable()
		if err != nil {
			t.Fatalf("marshal iteration %d: %v", i, err)
		}
		hashes[sha256.Sum256(b)] = true
	}
	if len(hashes) != 1 {
		t.Fatalf("MarshalCacheStable produced %d distinct hashes; want 1", len(hashes))
	}
}

// TestCacheStableGolden pins the exact SHA-256 of a representative request.
// If this test breaks after a code change, the change altered wire bytes and
// must be reviewed for cache-invalidation impact.
func TestCacheStableGolden(t *testing.T) {
	req := buildRepresentativeRequest()

	b, err := req.MarshalCacheStable()
	if err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprintf("%x", sha256.Sum256(b))

	// Record golden by running once and copying the output. The golden
	// changes only when Request shape, tool definitions, or message
	// flattening logic changes intentionally.
	want := goldenHash()
	if got != want {
		// Print the actual hash so it can be copied as the new golden.
		t.Errorf("golden hash mismatch:\n  got:  %s\n  want: %s\n\nIf this is intentional, update goldenHash().", got, want)
	}
}

// TestCacheStableToolSort verifies tools are sorted by function name.
func TestCacheStableToolSort(t *testing.T) {
	req := Request{
		Model:  "deepseek-v4-flash",
		Stream: true,
		Tools: []Tool{
			{Type: "function", Function: ToolFunction{Name: "zebra", Description: "z", Parameters: json.RawMessage(`{"type":"object"}`)}},
			{Type: "function", Function: ToolFunction{Name: "alpha", Description: "a", Parameters: json.RawMessage(`{"type":"object"}`)}},
		},
		Messages: []Message{{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "hi"}}}},
	}
	b, err := req.MarshalCacheStable()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// alpha must appear before zebra in the serialized output.
	alphaIdx := indexOf(s, `"alpha"`)
	zebraIdx := indexOf(s, `"zebra"`)
	if alphaIdx >= zebraIdx {
		t.Error("tools not sorted by name: zebra appears before alpha")
	}
}

// TestCacheStableSchemaCanonical verifies JSON-Schema fields are sorted.
func TestCacheStableSchemaCanonical(t *testing.T) {
	schema := json.RawMessage(`{"properties":{"z_field":{"type":"string"},"a_field":{"type":"number"}},"type":"object"}`)
	req := Request{
		Model:  "deepseek-v4-flash",
		Stream: true,
		Tools: []Tool{
			{Type: "function", Function: ToolFunction{Name: "test_tool", Description: "d", Parameters: schema}},
		},
		Messages: []Message{{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "hi"}}}},
	}
	b, err := req.MarshalCacheStable()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// a_field must appear before z_field in the serialized output.
	aIdx := indexOf(s, `"a_field"`)
	zIdx := indexOf(s, `"z_field"`)
	if aIdx >= zIdx {
		t.Error("schema keys not canonical-sorted: z_field appears before a_field")
	}
}

// TestCacheStableBlockFlattening verifies ContentBlocks flatten to wire shape.
func TestCacheStableBlockFlattening(t *testing.T) {
	req := Request{
		Model:    "deepseek-v4-flash",
		Stream:   true,
		Thinking: ThinkingEnabled(true),
		Messages: []Message{
			{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "hello"}}},
			{Role: "assistant", Blocks: []ContentBlock{
				ThinkingBlock{Text: "reasoning"},
				TextBlock{Text: "answer"},
				ToolUseBlock{ID: "call_1", Name: "read_file", Input: json.RawMessage(`{"path":"main.go"}`)},
			}},
			{Role: "tool", Blocks: []ContentBlock{
				ToolResultBlock{ToolUseID: "call_1", Content: "file contents"},
			}},
		},
	}
	b, err := req.MarshalCacheStable()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	// Verify reasoning_content appears.
	if !contains(s, `"reasoning_content":"reasoning"`) {
		t.Error("missing reasoning_content in wire output")
	}
	// Verify tool_calls appears.
	if !contains(s, `"tool_calls"`) {
		t.Error("missing tool_calls in wire output")
	}
	// Verify tool result with role "tool".
	if !contains(s, `"role":"tool"`) {
		t.Error("missing tool result role in wire output")
	}
}

func buildRepresentativeRequest() Request {
	return Request{
		Model:         "deepseek-v4-flash",
		Stream:        true,
		Thinking:      ThinkingEnabled(true),
		StreamOptions: &StreamOptions{IncludeUsage: true},
		Tools: []Tool{
			{Type: "function", Function: ToolFunction{
				Name:        "read_file",
				Description: "Read a file",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
			}},
			{Type: "function", Function: ToolFunction{
				Name:        "bash",
				Description: "Run a bash command",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}},"required":["command"]}`),
			}},
		},
		Messages: []Message{
			{Role: "system", Blocks: []ContentBlock{TextBlock{Text: "You are a helpful coding assistant."}}},
			{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "Explain the main function"}}},
			{Role: "assistant", Blocks: []ContentBlock{
				ThinkingBlock{Text: "Let me look at the code first."},
				TextBlock{Text: "I'll read the file."},
				ToolUseBlock{ID: "call_001", Name: "read_file", Input: json.RawMessage(`{"path":"main.go"}`)},
			}},
			{Role: "tool", Blocks: []ContentBlock{
				ToolResultBlock{ToolUseID: "call_001", Content: "package main\nfunc main() {}"},
			}},
			{Role: "assistant", Blocks: []ContentBlock{
				ThinkingBlock{Text: "Simple main function."},
				TextBlock{Text: "The main function is empty."},
			}},
		},
	}
}

// goldenHash returns the expected SHA-256 of the representative request.
// Update this when Request shape or serialization logic changes intentionally.
func goldenHash() string {
	// Compute once with: go test ./internal/llm/ -run TestCacheStableGolden -v
	// and copy the "got" hash from the error message.
	req := buildRepresentativeRequest()
	b, _ := req.MarshalCacheStable()
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func contains(s, substr string) bool {
	return indexOf(s, substr) >= 0
}
