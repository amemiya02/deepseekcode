package llm

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestFlattenForWire(t *testing.T) {
	cases := []struct {
		name   string
		in     Message
		assert func(t *testing.T, got wireMessage)
	}{
		{
			"legacy_only",
			Message{Role: "assistant", Content: "hi", ReasoningContent: "think", ToolCalls: []ToolCall{{ID: "x", Type: "function", Function: ToolCallFunc{Name: "bash", Arguments: "{}"}}}},
			func(t *testing.T, got wireMessage) {
				if got.Content != "hi" || got.ReasoningContent != "think" {
					t.Errorf("legacy fields not preserved: %+v", got)
				}
				if len(got.ToolCalls) != 1 || got.ToolCalls[0].ID != "x" {
					t.Errorf("legacy tool_calls not preserved: %+v", got.ToolCalls)
				}
			},
		},
		{
			"blocks_text_only",
			Message{Role: "assistant", Blocks: []ContentBlock{TextBlock{Text: "hello"}}},
			func(t *testing.T, got wireMessage) {
				if got.Content != "hello" {
					t.Errorf("want hello, got %q", got.Content)
				}
				if got.ReasoningContent != "" || len(got.ToolCalls) != 0 {
					t.Errorf("unexpected extras: %+v", got)
				}
			},
		},
		{
			"blocks_thinking_and_text",
			Message{Role: "assistant", Blocks: []ContentBlock{
				ThinkingBlock{Text: "plan"},
				TextBlock{Text: "ack"},
			}},
			func(t *testing.T, got wireMessage) {
				if got.ReasoningContent != "plan" {
					t.Errorf("reasoning_content: %q", got.ReasoningContent)
				}
				if got.Content != "ack" {
					t.Errorf("content: %q", got.Content)
				}
			},
		},
		{
			"blocks_tool_use_sorted",
			Message{Role: "assistant", Blocks: []ContentBlock{
				ToolUseBlock{ID: "c", Name: "x", Input: json.RawMessage(`{}`)},
				ToolUseBlock{ID: "a", Name: "y", Input: json.RawMessage(`{}`)},
				ToolUseBlock{ID: "b", Name: "z", Input: json.RawMessage(`{}`)},
			}},
			func(t *testing.T, got wireMessage) {
				if len(got.ToolCalls) != 3 {
					t.Fatalf("want 3 calls, got %d", len(got.ToolCalls))
				}
				want := []string{"a", "b", "c"}
				for i, id := range want {
					if got.ToolCalls[i].ID != id {
						t.Errorf("idx %d: want %q, got %q", i, id, got.ToolCalls[i].ID)
					}
				}
			},
		},
		{
			"blocks_empty",
			Message{Role: "user"},
			func(t *testing.T, got wireMessage) {
				if got.Role != "user" || got.Content != "" || got.ReasoningContent != "" || got.ToolCalls != nil {
					t.Errorf("want zero-value wire message, got %+v", got)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := flattenForWire(tc.in)
			tc.assert(t, a)

			// Cache stability: same input → same JSON bytes twice.
			b1, err := json.Marshal(flattenForWire(tc.in))
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			b2, err := json.Marshal(flattenForWire(tc.in))
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if !bytes.Equal(b1, b2) {
				t.Errorf("non-deterministic flatten:\n%s\nvs\n%s", b1, b2)
			}
		})
	}
}
