package session

import (
	"context"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// UsageSummary aggregates persisted assistant-message usage for a
// session, enabling resumed sessions to restore cost/cache totals.
type UsageSummary struct {
	Turns           int
	CacheHitTokens  int
	CacheMissTokens int
	OutputTokens    int
	CostYuan        float64
	SavedYuan       float64
}

// UsageSummary returns the aggregated usage from persisted assistant
// messages for the given session. Only rows with at least one usage
// token column > 0 are counted as turns.
func (s *Store) UsageSummary(ctx context.Context, sessionID string) (UsageSummary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT COALESCE(model,''),
		        COALESCE(cache_hit_tokens,0),
		        COALESCE(miss_tokens,0),
		        COALESCE(output_tokens,0),
		        COALESCE(cost_yuan,0)
		 FROM messages
		 WHERE session_id = ? AND role = 'assistant'
		   AND (cache_hit_tokens > 0 OR miss_tokens > 0 OR output_tokens > 0)
		 ORDER BY idx ASC`, sessionID)
	if err != nil {
		return UsageSummary{}, err
	}
	defer rows.Close()

	var sum UsageSummary
	for rows.Next() {
		var (
			model          string
			hit, miss, out int
			cost           float64
		)
		if err := rows.Scan(&model, &hit, &miss, &out, &cost); err != nil {
			return UsageSummary{}, err
		}
		sum.Turns++
		sum.CacheHitTokens += hit
		sum.CacheMissTokens += miss
		sum.OutputTokens += out
		sum.CostYuan += cost
		u := llm.Usage{
			PromptCacheHitTokens:  hit,
			PromptCacheMissTokens: miss,
			CompletionTokens:      out,
		}
		sum.SavedYuan += llm.CacheSavings(model, u)
	}
	return sum, rows.Err()
}

// UsageSummary is a convenience wrapper that calls the store method
// scoped to this persister's session.
func (p *Persister) UsageSummary(ctx context.Context) (UsageSummary, error) {
	return p.store.UsageSummary(ctx, p.sessID)
}
