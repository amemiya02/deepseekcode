package llm

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// TestCacheStableGolden pins the exact bytes of a representative request.
// The golden file lives in testdata/cache_stable.golden.json and is
// compared byte-for-byte — any serialization change will be caught as a
// diff. If the change is intentional, regenerate the golden by running:
//
//	UPDATE_GOLDEN=1 go test -run TestCacheStableGolden ./internal/llm/
func TestCacheStableGolden(t *testing.T) {
	req := buildRepresentativeRequest()

	b, err := req.MarshalCacheStable()
	if err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprintf("%x", sha256.Sum256(b))

	goldenPath := filepath.Join("testdata", "cache_stable.golden.json")

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, b, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("updated golden: %s (sha256=%s)", goldenPath, got)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v\nRegenerate with: UPDATE_GOLDEN=1 go test -run TestCacheStableGolden ./internal/llm/", err)
	}
	wantHash := fmt.Sprintf("%x", sha256.Sum256(want))

	if got != wantHash {
		t.Errorf("golden drift:\n  got:  %s\n  want: %s\n\nIf intentional, update with: UPDATE_GOLDEN=1 go test -run TestCacheStableGolden ./internal/llm/", got, wantHash)
	}

	// Also verify byte-identical for maximum diff clarity.
	if string(b) != string(want) {
		t.Errorf("golden bytes differ (len got=%d, want=%d)", len(b), len(want))
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
	alphaIdx := strings.Index(s, `"alpha"`)
	zebraIdx := strings.Index(s, `"zebra"`)
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
	aIdx := strings.Index(s, `"a_field"`)
	zIdx := strings.Index(s, `"z_field"`)
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
	if !strings.Contains(s, `"reasoning_content":"reasoning"`) {
		t.Error("missing reasoning_content in wire output")
	}
	// Verify tool_calls appears.
	if !strings.Contains(s, `"tool_calls"`) {
		t.Error("missing tool_calls in wire output")
	}
	// Verify tool result with role "tool".
	if !strings.Contains(s, `"role":"tool"`) {
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
