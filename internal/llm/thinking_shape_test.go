package llm

import (
	"bytes"
	"strings"
	"testing"
)

func TestThinkingSerializesAsStruct(t *testing.T) {
	req := Request{
		Model:    "deepseek-v4-flash",
		Messages: []Message{{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "hi"}}}},
		Thinking: ThinkingEnabled(true),
	}
	b, err := req.MarshalCacheStable()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Contains(b, []byte(`"thinking":{"type":"enabled"}`)) {
		t.Fatalf("thinking shape wrong; got: %s", b)
	}
}

// TestReasoningContentRoundtrips pins the fix for DeepSeek's 400
// "reasoning_content in the thinking mode must be passed back to the
// API." The assistant message's reasoning_content must serialize when
// non-empty so the model can continue the next turn coherently. When
// empty (thinking off), omitempty drops it.
func TestReasoningContentRoundtrips(t *testing.T) {
	req := Request{
		Model: "deepseek-v4-flash",
		Messages: []Message{
			{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "hi"}}},
			{Role: "assistant", Blocks: []ContentBlock{
				ThinkingBlock{Text: "let me think"},
				TextBlock{Text: "ok"},
			}},
		},
	}
	b, err := req.MarshalCacheStable()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Contains(b, []byte(`"reasoning_content":"let me think"`)) {
		t.Fatalf("reasoning_content missing from serialization: %s", b)
	}
}

func TestReasoningContentOmittedWhenEmpty(t *testing.T) {
	req := Request{
		Model: "deepseek-v4-flash",
		Messages: []Message{
			{Role: "assistant", Blocks: []ContentBlock{TextBlock{Text: "ok"}}},
		},
	}
	b, err := req.MarshalCacheStable()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), `"reasoning_content"`) {
		t.Fatalf("expected reasoning_content omitted when empty; got: %s", b)
	}
}

func TestThinkingNilOmitted(t *testing.T) {
	req := Request{
		Model:    "deepseek-v4-flash",
		Messages: []Message{{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "hi"}}}},
		Thinking: ThinkingEnabled(false), // nil
	}
	b, err := req.MarshalCacheStable()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), `"thinking"`) {
		t.Fatalf("expected no thinking field when disabled; got: %s", b)
	}
}
