// bench/cmd/cachedemo/report_test.go
package main

import (
	"math"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

func TestAggregateMatchesPricingTable(t *testing.T) {
	usages := []llm.Usage{
		{PromptCacheHitTokens: 8000, PromptCacheMissTokens: 300, CompletionTokens: 400},
		{PromptCacheHitTokens: 8000, PromptCacheMissTokens: 300, CompletionTokens: 400},
	}
	got := aggregate("stable", "deepseek-v4-flash", usages)

	if got.Turns != 2 {
		t.Fatalf("Turns = %d, want 2", got.Turns)
	}
	wantSum := llm.Usage{PromptCacheHitTokens: 16000, PromptCacheMissTokens: 600, CompletionTokens: 800}
	if got.CostCNY != llm.Cost("deepseek-v4-flash", wantSum) {
		t.Fatalf("CostCNY = %v, want %v", got.CostCNY, llm.Cost("deepseek-v4-flash", wantSum))
	}
	if got.SavingsCNY != llm.CacheSavings("deepseek-v4-flash", wantSum) {
		t.Fatalf("SavingsCNY mismatch")
	}
	wantRate := 16000.0 / 16600.0
	if math.Abs(got.HitRate-wantRate) > 1e-9 {
		t.Fatalf("HitRate = %v, want %v", got.HitRate, wantRate)
	}
}

func TestRenderComparisonShowsTimesCheaper(t *testing.T) {
	naive := ArmResult{Name: "naive", CostCNY: 0.0153}
	stable := ArmResult{Name: "stable", Model: "deepseek-v4-flash", CostCNY: 0.0032}
	out := renderComparison([]ArmResult{naive, stable})
	if !strings.Contains(out, "cheaper") {
		t.Fatalf("comparison must report an N× cheaper figure:\n%s", out)
	}
}
