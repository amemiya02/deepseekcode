package tui

import (
	"fmt"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// CostReportInput holds the data needed to render a /cost report.
type CostReportInput struct {
	Model        string
	Steps        int
	Usage        llm.Usage
	CostYuan     float64
	SavedYuan    float64
	ContextLimit int
	CostKnown    bool
}

// RenderCostReport returns deterministic plain text summarizing the
// current session's cost/cache metrics.
func RenderCostReport(in CostReportInput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "model %s\n", in.Model)
	fmt.Fprintf(&b, "%d steps\n", in.Steps)

	hit := llm.CacheHitRate(in.Usage) * 100
	fmt.Fprintf(&b, "cache %.1f%% | hit %d | miss %d | out %d\n",
		hit, in.Usage.PromptCacheHitTokens, in.Usage.PromptCacheMissTokens, in.Usage.CompletionTokens)

	if in.CostKnown || llm.CostKnown(in.Model) {
		fmt.Fprintf(&b, "cost ¥%.6f | saved ¥%.6f\n", in.CostYuan, in.SavedYuan)
	} else {
		fmt.Fprintf(&b, "cost unknown model\n")
	}

	if in.ContextLimit > 0 {
		ctxTokens := in.Usage.PromptCacheHitTokens + in.Usage.PromptCacheMissTokens + in.Usage.CompletionTokens
		pct := float64(ctxTokens) / float64(in.ContextLimit) * 100
		fmt.Fprintf(&b, "context %s/%s (%.0f%%)\n", humanTokens(ctxTokens), humanTokens(in.ContextLimit), pct)
	}

	return b.String()
}
