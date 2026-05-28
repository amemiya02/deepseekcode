package agent

import "github.com/amemiya02/deepseekcode/internal/llm"

const defaultProjectedOutputTokens = 2048

// ProjectedTurnCostCNY returns a conservative pre-stream estimate for one model
// turn. It assumes all prompt input misses the provider cache because this gate
// runs before DeepSeek returns authoritative cache hit/miss usage.
func ProjectedTurnCostCNY(model string, req llm.Request) float64 {
	if !llm.CostKnown(model) {
		return 0
	}
	outputTokens := req.MaxTokens
	if outputTokens <= 0 {
		outputTokens = defaultProjectedOutputTokens
	}
	return llm.Cost(model, llm.Usage{
		PromptCacheMissTokens: EstimateTokens(req.Messages),
		CompletionTokens:      outputTokens,
	})
}
