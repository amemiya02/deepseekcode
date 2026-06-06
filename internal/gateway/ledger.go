package gateway

import (
	"errors"
	"io/fs"
	"net/http"

	"github.com/amemiya02/deepseekcode/internal/traceinspect"
)

// LedgerRow is the SPA-facing per-turn cache row (GET /v1/cache/ledger). It
// mirrors traceinspect.TurnLedger with snake_case JSON field names.
type LedgerRow struct {
	Turn         int     `json:"turn"`
	EpochID      string  `json:"epoch_id"`
	HitTokens    int     `json:"hit_tokens"`
	MissTokens   int     `json:"miss_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostCNY      float64 `json:"cost_cny"`
	Evicted      bool    `json:"evicted"`
	Why          string  `json:"why"`
}

// handleCacheledger implements GET /v1/cache/ledger. It returns the per-turn
// eviction ledger from traceinspect.ExplainFile. A missing trace yields an
// empty rows array (200), never a 500 — matching /v1/cache's discipline.
func (h *Handler) handleCacheLedger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rows := []LedgerRow{}
	if h.tracePath != "" {
		ledger, err := traceinspect.ExplainFile(h.tracePath)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			http.Error(w, "ledger: "+err.Error(), http.StatusInternalServerError)
			return
		}
		for _, row := range ledger {
			rows = append(rows, LedgerRow{
				Turn: row.Turn, EpochID: row.EpochID, HitTokens: row.HitTokens,
				MissTokens: row.MissTokens, OutputTokens: row.OutputTokens,
				CostCNY: row.CostCNY, Evicted: row.Evicted, Why: row.Why,
			})
		}
	}
	writeJSON(w, map[string]any{"rows": rows})
}
