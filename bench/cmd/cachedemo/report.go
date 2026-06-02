// bench/cmd/cachedemo/report.go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// ArmResult is one arm's aggregated outcome, all dollars derived from the real
// pricing table in internal/llm/cache_metrics.go.
type ArmResult struct {
	Name       string  `json:"name"`
	Model      string  `json:"model"`
	Turns      int     `json:"turns"`
	HitTokens  int     `json:"hit_tokens"`
	MissTokens int     `json:"miss_tokens"`
	OutTokens  int     `json:"out_tokens"`
	HitRate    float64 `json:"hit_rate"`
	CostCNY    float64 `json:"cost_cny"`
	SavingsCNY float64 `json:"savings_cny"`
}

// aggregate sums per-turn usage and prices it with the canonical helpers, so
// the demo can never diverge from what the Cost HUD shows.
func aggregate(name, model string, usages []llm.Usage) ArmResult {
	var sum llm.Usage
	for _, u := range usages {
		sum.PromptCacheHitTokens += u.PromptCacheHitTokens
		sum.PromptCacheMissTokens += u.PromptCacheMissTokens
		sum.CompletionTokens += u.CompletionTokens
	}
	return ArmResult{
		Name:       name,
		Model:      model,
		Turns:      len(usages),
		HitTokens:  sum.PromptCacheHitTokens,
		MissTokens: sum.PromptCacheMissTokens,
		OutTokens:  sum.CompletionTokens,
		HitRate:    llm.CacheHitRate(sum),
		CostCNY:    llm.Cost(model, sum),
		SavingsCNY: llm.CacheSavings(model, sum),
	}
}

// renderComparison prints a per-arm table and, when exactly two arms are given
// (naive first, stable second), an "N× cheaper" headline.
func renderComparison(arms []ArmResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-8s %6s %10s %10s %8s %10s %10s\n",
		"arm", "turns", "hit_tok", "miss_tok", "hit%", "cost¥", "saved¥")
	for _, a := range arms {
		fmt.Fprintf(&b, "%-8s %6d %10d %10d %7.1f%% %10.5f %10.5f\n",
			a.Name, a.Turns, a.HitTokens, a.MissTokens, a.HitRate*100, a.CostCNY, a.SavingsCNY)
	}
	if len(arms) == 2 && arms[1].CostCNY > 0 {
		ratio := arms[0].CostCNY / arms[1].CostCNY
		fmt.Fprintf(&b, "\ndeepseekcode (%s) is %.1f× cheaper than the cache-naive baseline (%s) on %s.\n",
			arms[1].Name, ratio, arms[0].Name, arms[1].Model)
	}
	return b.String()
}

// writeJSON persists the arms for the website/README and reproducibility.
func writeJSON(path string, arms []ArmResult) error {
	data, err := json.MarshalIndent(arms, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
