package agent

import (
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/tokenizer"
)

func TestEstimateInputTokens_PrefersTokenizerWhenAvailable(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "hello world"}}},
	}
	if !tokenizer.Available() {
		t.Skip("tokenizer not available")
	}
	expected, _ := tokenizer.CountMessages(msgs)
	got := EstimateInputTokens(msgs, 4.0)
	if got != expected {
		t.Fatalf("EstimateInputTokens = %d, want %d (tokenizer)", got, expected)
	}
}

func TestEstimateInputTokens_FallbackContract(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "hello"}}},
	}
	got := EstimateInputTokens(msgs, 4.0)
	if got < 0 {
		t.Fatalf("EstimateInputTokens = %d, want >= 0", got)
	}
	// If tokenizer is available, should equal CountMessages.
	if tokenizer.Available() {
		expected, _ := tokenizer.CountMessages(msgs)
		if got != expected {
			t.Fatalf("EstimateInputTokens = %d, want %d", got, expected)
		}
	}
}
