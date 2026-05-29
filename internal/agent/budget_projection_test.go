package agent

import (
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

func TestProjectedTurnCostCNYUsesMissPricingConservatively(t *testing.T) {
	req := llm.Request{
		Model: "deepseek-v4-flash",
		Messages: []llm.Message{
			{Role: "system", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "system prompt"}}},
			{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "hello world"}}},
		},
		MaxTokens: 1000,
	}

	got := ProjectedTurnCostCNY("deepseek-v4-flash", req, defaultCharsPerToken)
	if got <= 0 {
		t.Fatalf("ProjectedTurnCostCNY = %v, want positive cost", got)
	}

	inputTokens := EstimateTokens(req.Messages)
	want := llm.Cost("deepseek-v4-flash", llm.Usage{
		PromptCacheMissTokens: inputTokens,
		CompletionTokens:      1000,
	})
	if got != want {
		t.Fatalf("ProjectedTurnCostCNY = %.12f, want %.12f", got, want)
	}
}

func TestProjectedTurnCostCNYDefaultOutputWhenMaxTokensUnset(t *testing.T) {
	req := llm.Request{
		Model: "deepseek-v4-flash",
		Messages: []llm.Message{
			{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "short"}}},
		},
	}

	got := ProjectedTurnCostCNY("deepseek-v4-flash", req, defaultCharsPerToken)
	inputTokens := EstimateTokens(req.Messages)
	want := llm.Cost("deepseek-v4-flash", llm.Usage{
		PromptCacheMissTokens: inputTokens,
		CompletionTokens:      defaultProjectedOutputTokens,
	})
	if got != want {
		t.Fatalf("ProjectedTurnCostCNY = %.12f, want %.12f", got, want)
	}
}

func TestProjectedTurnCostCNYUnknownModelReturnsZero(t *testing.T) {
	req := llm.Request{
		Model: "unknown-model",
		Messages: []llm.Message{
			{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "hello"}}},
		},
		MaxTokens: 1000,
	}

	if got := ProjectedTurnCostCNY("unknown-model", req, defaultCharsPerToken); got != 0 {
		t.Fatalf("ProjectedTurnCostCNY unknown model = %v, want 0", got)
	}
}
