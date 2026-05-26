package llm

import (
	"encoding/json"
	"testing"
)

func TestSanitizeForDeepSeek_ThinkingDisabled(t *testing.T) {
	req := Request{
		Model: "deepseek-v4-flash",
		Messages: []Message{
			{Role: "assistant", Blocks: []ContentBlock{
				ToolUseBlock{ID: "c1", Name: "read_file", Input: json.RawMessage(`{"path":"x"}`)},
			}},
		},
	}
	// Thinking is nil (disabled by default)
	out := req.SanitizeForDeepSeek()
	if len(out.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(out.Messages))
	}
	// Should NOT have thinking block prepended
	if len(out.Messages[0].Blocks) != 1 {
		t.Errorf("expected 1 block unchanged, got %d", len(out.Messages[0].Blocks))
	}
	if _, ok := out.Messages[0].Blocks[0].(ToolUseBlock); !ok {
		t.Errorf("expected ToolUseBlock, got %T", out.Messages[0].Blocks[0])
	}
}

func TestSanitizeForDeepSeek_ThinkingEnabled_NoToolUse(t *testing.T) {
	req := Request{
		Model:    "deepseek-v4-flash",
		Thinking: ThinkingEnabled(true),
		Messages: []Message{
			{Role: "assistant", Blocks: []ContentBlock{
				TextBlock{Text: "hello"},
			}},
		},
	}
	out := req.SanitizeForDeepSeek()
	if len(out.Messages[0].Blocks) != 1 {
		t.Errorf("text-only message should be unchanged, got %d blocks", len(out.Messages[0].Blocks))
	}
}

func TestSanitizeForDeepSeek_ThinkingEnabled_HasExistingThinking(t *testing.T) {
	req := Request{
		Model:    "deepseek-v4-flash",
		Thinking: ThinkingEnabled(true),
		Messages: []Message{
			{Role: "assistant", Blocks: []ContentBlock{
				ThinkingBlock{Text: "step 1"},
				ToolUseBlock{ID: "c1", Name: "read_file", Input: json.RawMessage(`{}`)},
			}},
		},
	}
	out := req.SanitizeForDeepSeek()
	if len(out.Messages[0].Blocks) != 2 {
		t.Errorf("message with existing thinking should be unchanged, got %d blocks", len(out.Messages[0].Blocks))
	}
}

func TestSanitizeForDeepSeek_ThinkingEnabled_NeedsPlaceholder(t *testing.T) {
	req := Request{
		Model:    "deepseek-v4-flash",
		Thinking: ThinkingEnabled(true),
		Messages: []Message{
			{Role: "assistant", Blocks: []ContentBlock{
				ToolUseBlock{ID: "c1", Name: "read_file", Input: json.RawMessage(`{"path":"README.md"}`)},
			}},
		},
	}
	out := req.SanitizeForDeepSeek()
	if len(out.Messages[0].Blocks) != 2 {
		t.Fatalf("expected 2 blocks (thinking + tool_use), got %d", len(out.Messages[0].Blocks))
	}
	tb, ok := out.Messages[0].Blocks[0].(ThinkingBlock)
	if !ok {
		t.Fatalf("expected ThinkingBlock first, got %T", out.Messages[0].Blocks[0])
	}
	if tb.Text != OmittedReasoningPlaceholder {
		t.Errorf("expected placeholder %q, got %q", OmittedReasoningPlaceholder, tb.Text)
	}
}

func TestSanitizeForDeepSeek_MultipleToolCalls_OnePlaceholder(t *testing.T) {
	req := Request{
		Model:    "deepseek-v4-flash",
		Thinking: ThinkingEnabled(true),
		Messages: []Message{
			{Role: "assistant", Blocks: []ContentBlock{
				ToolUseBlock{ID: "c1", Name: "read_file", Input: json.RawMessage(`{"path":"a"}`)},
				ToolUseBlock{ID: "c2", Name: "read_file", Input: json.RawMessage(`{"path":"b"}`)},
			}},
		},
	}
	out := req.SanitizeForDeepSeek()
	// 1 thinking + 2 tool_use = 3 blocks
	if len(out.Messages[0].Blocks) != 3 {
		t.Errorf("expected 3 blocks, got %d", len(out.Messages[0].Blocks))
	}
	// Count thinking blocks
	thinkingCount := 0
	for _, b := range out.Messages[0].Blocks {
		if _, ok := b.(ThinkingBlock); ok {
			thinkingCount++
		}
	}
	if thinkingCount != 1 {
		t.Errorf("expected exactly 1 thinking block, got %d", thinkingCount)
	}
}

func TestSanitizeForDeepSeek_Idempotent(t *testing.T) {
	req := Request{
		Model:    "deepseek-v4-flash",
		Thinking: ThinkingEnabled(true),
		Messages: []Message{
			{Role: "assistant", Blocks: []ContentBlock{
				ToolUseBlock{ID: "c1", Name: "read_file", Input: json.RawMessage(`{}`)},
			}},
		},
	}
	first := req.SanitizeForDeepSeek()
	second := first.SanitizeForDeepSeek()

	// Both should have same number of blocks
	if len(first.Messages[0].Blocks) != len(second.Messages[0].Blocks) {
		t.Errorf("sanitize not idempotent: first has %d blocks, second has %d",
			len(first.Messages[0].Blocks), len(second.Messages[0].Blocks))
	}
}

func TestSanitizeForDeepSeek_DoesNotModifyOtherRoles(t *testing.T) {
	req := Request{
		Model:    "deepseek-v4-flash",
		Thinking: ThinkingEnabled(true),
		Messages: []Message{
			{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "hi"}}},
			{Role: "tool", Blocks: []ContentBlock{ToolResultBlock{ToolUseID: "c1", Content: "result"}}},
		},
	}
	out := req.SanitizeForDeepSeek()
	if len(out.Messages[0].Blocks) != 1 {
		t.Errorf("user message should be unchanged")
	}
	if len(out.Messages[1].Blocks) != 1 {
		t.Errorf("tool message should be unchanged")
	}
}

func TestMarshalCacheStable_WithSanitize_ReasoningContentPresent(t *testing.T) {
	req := Request{
		Model:    "deepseek-v4-flash",
		Stream:   true,
		Thinking: ThinkingEnabled(true),
		Messages: []Message{
			{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "read"}}},
			{Role: "assistant", Blocks: []ContentBlock{
				ToolUseBlock{ID: "c1", Name: "read_file", Input: json.RawMessage(`{"path":"README.md"}`)},
			}},
		},
	}
	// Sanitize before marshal
	sanitized := req.SanitizeForDeepSeek()
	data, err := sanitized.MarshalCacheStable()
	if err != nil {
		t.Fatalf("MarshalCacheStable: %v", err)
	}

	// Parse and check assistant message has reasoning_content
	var parsed struct {
		Messages []struct {
			Role             string `json:"role"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []struct {
				ID   string `json:"id"`
				Type string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse output: %v", err)
	}

	// Find assistant message
	var foundAssistant bool
	for _, m := range parsed.Messages {
		if m.Role == "assistant" {
			foundAssistant = true
			if m.ReasoningContent != OmittedReasoningPlaceholder {
				t.Errorf("expected reasoning_content %q, got %q", OmittedReasoningPlaceholder, m.ReasoningContent)
			}
			if len(m.ToolCalls) != 1 {
				t.Errorf("expected 1 tool_call, got %d", len(m.ToolCalls))
			}
		}
	}
	if !foundAssistant {
		t.Error("no assistant message found in output")
	}
}

func TestMarshalCacheStable_ThinkingDisabled_NoReasoningContent(t *testing.T) {
	req := Request{
		Model:  "deepseek-v4-flash",
		Stream: true,
		// Thinking is nil (disabled)
		Messages: []Message{
			{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "read"}}},
			{Role: "assistant", Blocks: []ContentBlock{
				ToolUseBlock{ID: "c1", Name: "read_file", Input: json.RawMessage(`{"path":"README.md"}`)},
			}},
		},
	}
	sanitized := req.SanitizeForDeepSeek()
	data, err := sanitized.MarshalCacheStable()
	if err != nil {
		t.Fatalf("MarshalCacheStable: %v", err)
	}

	// Parse and check assistant message has NO reasoning_content
	var parsed struct {
		Messages []struct {
			Role             string `json:"role"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse output: %v", err)
	}

	for _, m := range parsed.Messages {
		if m.Role == "assistant" && m.ReasoningContent != "" {
			t.Errorf("thinking-disabled request should not have reasoning_content, got %q", m.ReasoningContent)
		}
	}
}

func TestMarshalCacheStable_Deterministic(t *testing.T) {
	req := Request{
		Model:    "deepseek-v4-flash",
		Stream:   true,
		Thinking: ThinkingEnabled(true),
		Messages: []Message{
			{Role: "assistant", Blocks: []ContentBlock{
				ToolUseBlock{ID: "c1", Name: "read_file", Input: json.RawMessage(`{"path":"x"}`)},
			}},
		},
	}
	sanitized := req.SanitizeForDeepSeek()

	first, err := sanitized.MarshalCacheStable()
	if err != nil {
		t.Fatalf("first marshal: %v", err)
	}
	second, err := sanitized.MarshalCacheStable()
	if err != nil {
		t.Fatalf("second marshal: %v", err)
	}

	if string(first) != string(second) {
		t.Errorf("MarshalCacheStable not deterministic:\nfirst:  %s\nsecond: %s", first, second)
	}
}
