// headtohead.go extends the cachedemo harness with a four-cause cost
// attribution report. It consumes []cache.CacheReceipt (produced by the
// agent's per-turn cache.Attribute call) and aggregates them into per-cause
// token + yuan totals with a deterministic text report.
package main

import (
	"fmt"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/cache"
)

// causeRow holds aggregated metrics for one cache cause.
type causeRow struct {
	cause      cache.Cause
	turns      int
	hitTokens  int
	missTokens int
	costCNY    float64
	savedCNY   float64
}

// headToHeadReport aggregates receipts into a four-cause breakdown and
// returns a deterministic text report. The report contains lines for each
// cause plus a TOTAL line. This is the success gate for the cache cost
// engine -- it proves the four-cause attribution works end-to-end.
func headToHeadReport(label string, receipts []cache.CacheReceipt) string {
	rows := make(map[cache.Cause]*causeRow)
	for _, r := range receipts {
		row, ok := rows[r.Dominant]
		if !ok {
			row = &causeRow{cause: r.Dominant}
			rows[r.Dominant] = row
		}
		row.turns++
		row.hitTokens += r.HitTokens
		row.missTokens += r.MissTokens
		row.costCNY += r.CostCNY
		row.savedCNY += r.SavedCNY
	}

	// Stable cause order for deterministic output.
	causeOrder := []cache.Cause{
		cache.CauseColdFirst,
		cache.CausePrefixMut,
		cache.CauseResidualTail,
		cache.CauseCompactReset,
		cache.CauseSteady,
	}

	var b strings.Builder
	fmt.Fprintf(&b, "=== %s: four-cause cache cost breakdown ===\n", label)
	fmt.Fprintf(&b, "%-15s %5s %10s %10s %12s %12s\n",
		"cause", "turns", "hit_tok", "miss_tok", "cost¥", "saved¥")

	var totalHit, totalMiss int
	var totalCost, totalSaved float64
	for _, c := range causeOrder {
		row, ok := rows[c]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "%-15s %5d %10d %10d %12.6f %12.6f\n",
			row.cause, row.turns, row.hitTokens, row.missTokens, row.costCNY, row.savedCNY)
		totalHit += row.hitTokens
		totalMiss += row.missTokens
		totalCost += row.costCNY
		totalSaved += row.savedCNY
	}
	fmt.Fprintf(&b, "%-15s %5d %10d %10d %12.6f %12.6f\n",
		"TOTAL", len(receipts), totalHit, totalMiss, totalCost, totalSaved)

	return b.String()
}
