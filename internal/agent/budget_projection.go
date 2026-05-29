package agent

import "github.com/amemiya02/deepseekcode/internal/llm"

const defaultProjectedOutputTokens = 2048

// ProjectedTurnCostCNY returns a conservative pre-stream estimate for one model
// turn. It assumes all prompt input misses the provider cache because this gate
// runs before DeepSeek returns authoritative cache hit/miss usage. The input
// token count uses the per-session calibrated chars-per-token ratio
// (charsPerToken<=0 falls back to the cold-start char/4 prior), so the gate
// tracks the real prompt size on CJK/code-heavy sessions instead of the fixed
// char/4 estimate (T4.1).
func ProjectedTurnCostCNY(model string, req llm.Request, charsPerToken float64) float64 {
	if !llm.CostKnown(model) {
		return 0
	}
	outputTokens := req.MaxTokens
	if outputTokens <= 0 {
		outputTokens = defaultProjectedOutputTokens
	}
	return llm.Cost(model, llm.Usage{
		PromptCacheMissTokens: EstimateTokensCalibrated(req.Messages, charsPerToken),
		CompletionTokens:      outputTokens,
	})
}
