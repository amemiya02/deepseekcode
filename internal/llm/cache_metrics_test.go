package llm

import (
	"math"
	"testing"
)

func TestCostFlash(t *testing.T) {
	u := Usage{
		PromptCacheHitTokens:  100_000,
		PromptCacheMissTokens: 100_000,
		CompletionTokens:      50_000,
	}
	got := Cost("deepseek-v4-flash", u)
	// 100k * 0.02 + 100k * 1.0 + 50k * 2.0 all / 1M
	// = 2 + 100_000 + 100_000 = 200_002 / 1_000_000 = 0.200002
	want := (100_000*0.02 + 100_000*1.0 + 50_000*2.0) / 1_000_000.0
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("Cost flash = %v want %v", got, want)
	}
}

func TestCostPro(t *testing.T) {
	u := Usage{
		PromptCacheHitTokens:  10_000,
		PromptCacheMissTokens: 10_000,
		CompletionTokens:      5_000,
	}
	got := Cost("deepseek-v4-pro", u)
	want := (10_000*0.025 + 10_000*3.0 + 5_000*6.0) / 1_000_000.0
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("Cost pro = %v want %v", got, want)
	}
}

func TestCostUnknownModel(t *testing.T) {
	if Cost("unknown-model", Usage{CompletionTokens: 1000}) != 0 {
		t.Error("unknown model should cost 0")
	}
}

func TestPricesGolden(t *testing.T) {
	// Pin all 12 pricing values to prevent accidental drift.
	expected := map[string]Pricing{
		"deepseek-v4-flash": {InputCacheHit: 0.02, InputCacheMiss: 1.0, Output: 2.0},
		"deepseek-v4-pro":   {InputCacheHit: 0.025, InputCacheMiss: 3.0, Output: 6.0},
		"deepseek-chat":     {InputCacheHit: 0.02, InputCacheMiss: 1.0, Output: 2.0},
		"deepseek-reasoner": {InputCacheHit: 0.02, InputCacheMiss: 1.0, Output: 2.0},
	}

	if len(Prices) != 4 {
		t.Fatalf("len(Prices) = %d, want 4 (add audit entry if intentional)", len(Prices))
	}

	for model, want := range expected {
		got, ok := Prices[model]
		if !ok {
			t.Fatalf("Prices[%q] missing", model)
		}
		if got.InputCacheHit != want.InputCacheHit {
			t.Errorf("Prices[%q].InputCacheHit = %v, want %v", model, got.InputCacheHit, want.InputCacheHit)
		}
		if got.InputCacheMiss != want.InputCacheMiss {
			t.Errorf("Prices[%q].InputCacheMiss = %v, want %v", model, got.InputCacheMiss, want.InputCacheMiss)
		}
		if got.Output != want.Output {
			t.Errorf("Prices[%q].Output = %v, want %v", model, got.Output, want.Output)
		}
	}
}

func TestCostUnknownModelReturnsZero(t *testing.T) {
	u := Usage{PromptCacheHitTokens: 100, PromptCacheMissTokens: 100, CompletionTokens: 100}
	for _, model := range []string{"deepseek-ai/deepseek-v4-pro", "gpt-4o", ""} {
		if got := Cost(model, u); got != 0 {
			t.Errorf("Cost(%q, ...) = %v, want 0", model, got)
		}
	}
}

func TestCostCacheHitPath(t *testing.T) {
	// Verify cache-hit-only billing: 1M hit tokens at 0.025/1M = 0.025
	got := Cost("deepseek-v4-pro", Usage{PromptCacheHitTokens: 1_000_000})
	if got != 0.025 {
		t.Errorf("Cost(pro, 1M hits) = %v, want 0.025", got)
	}
}

func TestCacheHitRate(t *testing.T) {
	cases := []struct {
		hit, miss int
		want      float64
	}{
		{0, 0, 0},
		{100, 0, 1.0},
		{0, 100, 0.0},
		{75, 25, 0.75},
	}
	for _, c := range cases {
		got := CacheHitRate(Usage{
			PromptCacheHitTokens:  c.hit,
			PromptCacheMissTokens: c.miss,
		})
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("hit=%d miss=%d → %v want %v", c.hit, c.miss, got, c.want)
		}
	}
}

func TestCacheSavings(t *testing.T) {
	cases := []struct {
		name  string
		model string
		hits  int
		want  float64
	}{
		// flash: (miss 1.0 − hit 0.02) ¥/1M × 1M hit tokens = 0.98
		{"flash 1M hits", "deepseek-v4-flash", 1_000_000, 0.98},
		// pro: (miss 3.0 − hit 0.025) × 1M = 2.975
		{"pro 1M hits", "deepseek-v4-pro", 1_000_000, 2.975},
		{"no hits → 0", "deepseek-v4-flash", 0, 0},
		{"unknown model → 0", "gpt-4o", 1_000_000, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := CacheSavings(c.model, Usage{PromptCacheHitTokens: c.hits})
			if math.Abs(got-c.want) > 1e-9 {
				t.Errorf("CacheSavings(%q, hits=%d) = %v, want %v", c.model, c.hits, got, c.want)
			}
		})
	}
	// Savings must never exceed the all-miss cost for the same tokens (it is
	// the differential, not the full miss price).
	u := Usage{PromptCacheHitTokens: 500_000}
	missCost := Cost("deepseek-v4-flash", Usage{PromptCacheMissTokens: 500_000})
	if s := CacheSavings("deepseek-v4-flash", u); s >= missCost {
		t.Errorf("savings %v should be below all-miss cost %v", s, missCost)
	}
}

func TestCostKnown(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		{"deepseek-v4-pro", true},
		{"deepseek-v4-flash", true},
		{"deepseek-chat", true},
		{"deepseek-reasoner", true},
		{"deepseek-ai/deepseek-v4-pro", false},
		{"gpt-4o", false},
		{"", false},
	}
	for _, c := range cases {
		if got := CostKnown(c.model); got != c.want {
			t.Errorf("CostKnown(%q) = %v, want %v", c.model, got, c.want)
		}
	}
}
