// internal/llm/reasoning_policy_test.go
package llm

import (
	"testing"
)

func TestReadPolicy_Unset(t *testing.T) {
	t.Setenv("DEEPSEEKCODE_REASONING_RETAIN", "")
	t.Setenv("DEEPSEEKCODE_REASONING_DROP", "")
	p, err := ReadPolicy()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Mode != PolicyPassThrough {
		t.Fatalf("expected PassThrough, got %v", p.Mode)
	}
}

func TestReadPolicy_Drop(t *testing.T) {
	t.Setenv("DEEPSEEKCODE_REASONING_DROP", "1")
	t.Setenv("DEEPSEEKCODE_REASONING_RETAIN", "")
	p, err := ReadPolicy()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Mode != PolicyDropAll {
		t.Fatalf("expected DropAll, got %v", p.Mode)
	}
}

func TestReadPolicy_Retain(t *testing.T) {
	t.Setenv("DEEPSEEKCODE_REASONING_DROP", "")
	t.Setenv("DEEPSEEKCODE_REASONING_RETAIN", "3")
	p, err := ReadPolicy()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Mode != PolicyRetainLast {
		t.Fatalf("expected RetainLast, got %v", p.Mode)
	}
	if p.N != 3 {
		t.Fatalf("expected N=3, got %d", p.N)
	}
}

func TestReadPolicy_RetainInvalid(t *testing.T) {
	t.Setenv("DEEPSEEKCODE_REASONING_DROP", "")
	t.Setenv("DEEPSEEKCODE_REASONING_RETAIN", "0")
	_, err := ReadPolicy()
	if err == nil {
		t.Fatal("expected error for N=0")
	}
}

func TestReadPolicy_BothSet(t *testing.T) {
	t.Setenv("DEEPSEEKCODE_REASONING_DROP", "1")
	t.Setenv("DEEPSEEKCODE_REASONING_RETAIN", "2")
	_, err := ReadPolicy()
	if err == nil {
		t.Fatal("expected error when both env vars set")
	}
}

// --- applyPolicy tests ---
// NOTE: The plan's spec uses Content []ContentBlock but the real Message struct
// uses Blocks []ContentBlock. Tests are adapted to match the actual type.

func makeAssistantWithReasoning(text, reasoning string) Message {
	blocks := []ContentBlock{}
	if reasoning != "" {
		blocks = append(blocks, ThinkingBlock{Text: reasoning})
	}
	blocks = append(blocks, TextBlock{Text: text})
	return Message{Role: "assistant", Blocks: blocks}
}

func reasoningOf(m Message) string {
	for _, b := range m.Blocks {
		if tb, ok := b.(ThinkingBlock); ok {
			return tb.Text
		}
	}
	return ""
}

func TestApplyPolicy_PassThrough(t *testing.T) {
	msgs := []Message{
		makeAssistantWithReasoning("hello", "think1"),
		makeAssistantWithReasoning("world", "think2"),
	}
	out := applyPolicy(msgs, ReasoningPolicy{Mode: PolicyPassThrough})
	if reasoningOf(out[0]) != "think1" || reasoningOf(out[1]) != "think2" {
		t.Fatal("PassThrough must not modify reasoning content")
	}
}

func TestApplyPolicy_DropAll(t *testing.T) {
	msgs := []Message{
		makeAssistantWithReasoning("hello", "think1"),
		makeAssistantWithReasoning("world", "think2"),
	}
	out := applyPolicy(msgs, ReasoningPolicy{Mode: PolicyDropAll})
	if reasoningOf(out[0]) != "" || reasoningOf(out[1]) != "" {
		t.Fatal("DropAll must remove all ThinkingBlocks")
	}
}

func TestApplyPolicy_RetainLast1(t *testing.T) {
	msgs := []Message{
		makeAssistantWithReasoning("a", "thinkA"),
		makeAssistantWithReasoning("b", "thinkB"),
		makeAssistantWithReasoning("c", "thinkC"),
	}
	out := applyPolicy(msgs, ReasoningPolicy{Mode: PolicyRetainLast, N: 1})
	// only last assistant turn keeps reasoning
	if reasoningOf(out[0]) != OmittedReasoningPlaceholder {
		t.Fatalf("turn 0 should be placeholder, got %q", reasoningOf(out[0]))
	}
	if reasoningOf(out[1]) != OmittedReasoningPlaceholder {
		t.Fatalf("turn 1 should be placeholder, got %q", reasoningOf(out[1]))
	}
	if reasoningOf(out[2]) != "thinkC" {
		t.Fatalf("last turn should keep reasoning, got %q", reasoningOf(out[2]))
	}
}

func TestApplyPolicy_RetainLast2_NonAssistantUntouched(t *testing.T) {
	userMsg := Message{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "hi"}}}
	msgs := []Message{
		makeAssistantWithReasoning("a", "thinkA"),
		userMsg,
		makeAssistantWithReasoning("b", "thinkB"),
		userMsg,
		makeAssistantWithReasoning("c", "thinkC"),
	}
	out := applyPolicy(msgs, ReasoningPolicy{Mode: PolicyRetainLast, N: 2})
	// assistant turns: indices 0, 2, 4 in original → last 2 are indices 2 and 4
	if reasoningOf(out[0]) != OmittedReasoningPlaceholder {
		t.Fatalf("first assistant turn should be placeholder")
	}
	if reasoningOf(out[2]) != "thinkB" {
		t.Fatalf("second assistant turn (N=2 window) should keep reasoning")
	}
	if reasoningOf(out[4]) != "thinkC" {
		t.Fatalf("third assistant turn should keep reasoning")
	}
}

func TestApplyPolicy_DoesNotMutateInput(t *testing.T) {
	msgs := []Message{
		makeAssistantWithReasoning("hello", "secret"),
	}
	_ = applyPolicy(msgs, ReasoningPolicy{Mode: PolicyDropAll})
	if reasoningOf(msgs[0]) != "secret" {
		t.Fatal("applyPolicy must not mutate input slice")
	}
}

func TestApplyPolicy_PassThroughNoReasoningUnchanged(t *testing.T) {
	// messages with no ThinkingBlocks must pass through byte-identically
	msgs := []Message{
		{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "q"}}},
		{Role: "assistant", Blocks: []ContentBlock{TextBlock{Text: "a"}}},
	}
	out := applyPolicy(msgs, ReasoningPolicy{Mode: PolicyPassThrough})
	if len(out) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(out))
	}
}
