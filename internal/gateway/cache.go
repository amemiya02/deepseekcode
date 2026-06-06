package gateway

import (
	"errors"
	"io/fs"

	"github.com/amemiya02/deepseekcode/internal/traceinspect"
)

// CacheReport mirrors the SPA's CacheReport interface (web/src/lib/api.ts).
// The JSON field names and types are a frozen contract with the SPA; do not
// rename or reorder without updating the SPA in lockstep.
type CacheReport struct {
	TotalUsageTurns   int     `json:"total_usage_turns"`
	CacheHitTokens    int     `json:"cache_hit_tokens"`
	CacheMissTokens   int     `json:"cache_miss_tokens"`
	OutputTokens      int     `json:"output_tokens"`
	CostCNY           float64 `json:"cost_cny"`
	FullBodyEvictions int     `json:"full_body_evictions"`
	MaxMissTokens     int     `json:"max_miss_tokens"`
	CacheHitRate      float64 `json:"cache_hit_rate"`
}

// buildCacheReport reads tracePath via traceinspect and assembles the
// SPA-facing CacheReport. A missing trace file is NOT an error: it yields a
// zero-valued report so /v1/cache never 500s before any run has produced a
// trace. Any other read/parse error is returned to the caller.
//
// The six aggregate fields come from traceinspect.InspectFile. The two
// eviction-derived fields (full_body_evictions, max_miss_tokens) come from the
// per-turn ledger produced by traceinspect.ExplainFile: an evicted turn is one
// flagged by the cache-doctor's eviction classifier, and max_miss_tokens is the
// largest single-turn miss seen across the run.
func buildCacheReport(tracePath string) (CacheReport, error) {
	if tracePath == "" {
		return CacheReport{}, nil
	}

	rep, err := traceinspect.InspectFile(tracePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// No trace yet: a zero-valued report is the correct answer.
			return CacheReport{}, nil
		}
		return CacheReport{}, err
	}

	out := CacheReport{
		TotalUsageTurns: rep.TotalUsageTurns,
		CacheHitTokens:  rep.CacheHitTokens,
		CacheMissTokens: rep.CacheMissTokens,
		OutputTokens:    rep.OutputTokens,
		CostCNY:         rep.CostCNY,
		CacheHitRate:    rep.CacheHitRate,
	}

	// Derive eviction stats from the per-turn ledger. If the ledger read fails
	// for a reason other than a missing file, surface it; a missing file was
	// already handled above (InspectFile would have returned the same error).
	ledger, err := traceinspect.ExplainFile(tracePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return out, nil
		}
		return CacheReport{}, err
	}
	for _, row := range ledger {
		if row.Evicted {
			out.FullBodyEvictions++
		}
		if row.MissTokens > out.MaxMissTokens {
			out.MaxMissTokens = row.MissTokens
		}
	}

	return out, nil
}
