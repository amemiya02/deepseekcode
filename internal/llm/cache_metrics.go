package llm

// Pricing is per million tokens. CNY (元) per million tokens, as
// published at https://api-docs.deepseek.com/zh-cn/quick_start/pricing
// and locked at design-doc time (2026-05). When DeepSeek changes
// pricing, update this table.
// 永久价说明与官方源核对见 docs/pricing.md。
type Pricing struct {
	InputCacheHit  float64 // ¥ / 1M tokens
	InputCacheMiss float64
	Output         float64
}

// Prices is the v0.1 default table. Hardcoded because:
//   - We want correct ¥ figures in the Cost HUD without an extra round
//     trip to a pricing endpoint.
//   - Updates happen at our cadence, not silently from the API.
var Prices = map[string]Pricing{
	"deepseek-v4-flash": {InputCacheHit: 0.02, InputCacheMiss: 1.0, Output: 2.0},
	"deepseek-v4-pro":   {InputCacheHit: 0.025, InputCacheMiss: 3.0, Output: 6.0},
	"deepseek-chat":     {InputCacheHit: 0.02, InputCacheMiss: 1.0, Output: 2.0}, // alias → flash
	"deepseek-reasoner": {InputCacheHit: 0.02, InputCacheMiss: 1.0, Output: 2.0}, // alias → flash thinking
}

// Cost returns the ¥ cost of one Usage record under the given model's
// pricing. Returns 0 if the model is unknown (we display "?" then).
func Cost(model string, u Usage) float64 {
	p, ok := Prices[model]
	if !ok {
		return 0
	}
	hits := float64(u.PromptCacheHitTokens)
	misses := float64(u.PromptCacheMissTokens)
	out := float64(u.CompletionTokens)
	const million = 1_000_000.0
	return (hits*p.InputCacheHit + misses*p.InputCacheMiss + out*p.Output) / million
}

// CostKnown reports whether model has a pricing entry (i.e., Cost returning
// 0 means "truly free" rather than "unknown model").
func CostKnown(model string) bool {
	_, ok := Prices[model]
	return ok
}

// CacheSavings returns the ¥ saved by the prompt cache for one Usage
// record: the cache-hit input tokens valued at the (miss − hit) price
// differential — i.e. what those tokens would have cost at the full
// cache-miss rate minus what they actually cost. Returns 0 for unknown
// models and is clamped at 0 (miss ≥ hit in every published tier, so a
// negative differential would only arise from a malformed price table).
func CacheSavings(model string, u Usage) float64 {
	p, ok := Prices[model]
	if !ok {
		return 0
	}
	saved := float64(u.PromptCacheHitTokens) * (p.InputCacheMiss - p.InputCacheHit)
	if saved < 0 {
		return 0
	}
	const million = 1_000_000.0
	return saved / million
}

// CacheHitRate returns the cache-hit fraction in [0,1] over input tokens.
// Returns 0 when there are no input tokens.
func CacheHitRate(u Usage) float64 {
	total := u.PromptCacheHitTokens + u.PromptCacheMissTokens
	if total == 0 {
		return 0
	}
	return float64(u.PromptCacheHitTokens) / float64(total)
}
