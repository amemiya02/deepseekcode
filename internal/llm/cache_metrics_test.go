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
