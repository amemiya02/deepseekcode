package agent

import "github.com/amemiya02/deepseekcode/internal/llm"

const defaultProjectedOutputTokens = 2048

// ProjectedTurnCostCNY returns a pre-stream cost estimate for one model turn.
// It runs before DeepSeek returns authoritative cache hit/miss usage, so it
// prices the prompt against the rolling session cache-hit rate (T4.2): the
// fraction cacheHitRate of input tokens is priced at the cheap cache-hit rate
// and the rest as cache miss.
//
// The input token count uses the per-session calibrated chars-per-token ratio
// (charsPerToken<=0 falls back to the cold-start char/4 prior), so the gate
// tracks the real prompt size on CJK/code-heavy sessions (T4.1).
//
// staticResidualTokens is the learned server-side static-prefix token cost
// (system+tool-schema template) not visible to the local tokenizer. Zero
// until learned from the first real usage frame (see Agent.learnStaticResidual).
//
// cacheHitRate is clamped to [0,1] and the hit-token split is floored, so the
// projection is biased toward cache miss. cacheHitRate==0 (the cold-start
// floor, before any usage is observed) reproduces the all-miss estimate
// exactly — the gate is never looser than the conservative default.
func ProjectedTurnCostCNY(model string, req llm.Request, charsPerToken, cacheHitRate float64, staticResidualTokens int) float64 {
	if !llm.CostKnown(model) {
		return 0
	}
	outputTokens := req.MaxTokens
	if outputTokens <= 0 {
		outputTokens = defaultProjectedOutputTokens
	}

	if cacheHitRate < 0 {
		cacheHitRate = 0
	}
	if cacheHitRate > 1 {
		cacheHitRate = 1
	}
	inputTokens := EstimateInputTokens(req.Messages, charsPerToken) + staticResidualTokens
	hitTokens := int(float64(inputTokens) * cacheHitRate) // floor → conservative bias toward miss
	missTokens := inputTokens - hitTokens

	return llm.Cost(model, llm.Usage{
		PromptCacheHitTokens:  hitTokens,
		PromptCacheMissTokens: missTokens,
		CompletionTokens:      outputTokens,
	})
}
