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

	got := ProjectedTurnCostCNY("deepseek-v4-flash", req, defaultCharsPerToken, 0)
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

	got := ProjectedTurnCostCNY("deepseek-v4-flash", req, defaultCharsPerToken, 0)
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

	if got := ProjectedTurnCostCNY("unknown-model", req, defaultCharsPerToken, 0); got != 0 {
		t.Fatalf("ProjectedTurnCostCNY unknown model = %v, want 0", got)
	}
}

// T4.2 — the projection discounts input by the rolling cache-hit rate. These
// pin the two contract boundaries exactly (cold-start==all-miss is the
// conservatism floor; full-hit==all-hit), that the rate is monotonic in
// between, and that out-of-range rates clamp instead of producing negative
// miss tokens.
func TestProjectedTurnCostCNYHitRateBlend(t *testing.T) {
	req := llm.Request{
		Model:     "deepseek-v4-flash",
		Messages:  msgsWithChars(40_000), // ~10k input tokens at char/4
		MaxTokens: 1000,
	}
	const model = "deepseek-v4-flash"
	inputTokens := EstimateTokensCalibrated(req.Messages, defaultCharsPerToken)

	allMiss := llm.Cost(model, llm.Usage{PromptCacheMissTokens: inputTokens, CompletionTokens: 1000})
	allHit := llm.Cost(model, llm.Usage{PromptCacheHitTokens: inputTokens, CompletionTokens: 1000})

	// (a) cold-start rate 0 reproduces the all-miss floor exactly.
	if got := ProjectedTurnCostCNY(model, req, defaultCharsPerToken, 0); got != allMiss {
		t.Errorf("hitRate 0 = %.12f, want all-miss %.12f", got, allMiss)
	}

	// (b) full hit rate 1.0 reproduces the all-hit cost exactly (input tokens
	// is an exact integer, so floor(input*1.0)==input leaves zero miss tokens).
	if got := ProjectedTurnCostCNY(model, req, defaultCharsPerToken, 1.0); got != allHit {
		t.Errorf("hitRate 1.0 = %.12f, want all-hit %.12f", got, allHit)
	}

	// (c) a mid rate sits strictly between the two and is cheaper than all-miss.
	half := ProjectedTurnCostCNY(model, req, defaultCharsPerToken, 0.5)
	if !(half < allMiss && half > allHit) {
		t.Errorf("hitRate 0.5 = %.12f, want strictly between all-hit %.12f and all-miss %.12f", half, allHit, allMiss)
	}

	// (d) clamping: a rate above 1 behaves like 1.0, a rate below 0 like 0.
	if got := ProjectedTurnCostCNY(model, req, defaultCharsPerToken, 1.5); got != allHit {
		t.Errorf("hitRate 1.5 should clamp to 1.0: got %.12f, want %.12f", got, allHit)
	}
	if got := ProjectedTurnCostCNY(model, req, defaultCharsPerToken, -0.5); got != allMiss {
		t.Errorf("hitRate -0.5 should clamp to 0: got %.12f, want %.12f", got, allMiss)
	}
}
