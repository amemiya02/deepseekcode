package gateway

import (
	"errors"
	"io/fs"
	"net/http"

	"github.com/amemiya02/deepseekcode/internal/traceinspect"
)

// timelineTurn is one turn row inside an epoch group (snake_case JSON).
type timelineTurn struct {
	Turn         int     `json:"turn"`
	HitTokens    int     `json:"hit_tokens"`
	MissTokens   int     `json:"miss_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostCNY      float64 `json:"cost_cny"`
	Evicted      bool    `json:"evicted"`
	Why          string  `json:"why"`
}

// timelineEpoch groups consecutive turns sharing an epoch id.
type timelineEpoch struct {
	EpochID string         `json:"epoch_id"`
	Turns   []timelineTurn `json:"turns"`
}

// handleSessionTimeline implements GET /v1/sessions/{id}/timeline. It returns
// the per-turn ledger from traceinspect.ExplainFile grouped into epochs. A
// missing trace yields an empty epochs array (200), never a 500 (matching
// /v1/cache discipline). The {id} is accepted but not yet used to filter (the
// single-process gateway has one active trace); a later wave keys per session.
func (h *Handler) handleSessionTimeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	epochs := []timelineEpoch{}
	if h.tracePath != "" {
		rows, err := traceinspect.ExplainFile(h.tracePath)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			http.Error(w, "timeline: "+err.Error(), http.StatusInternalServerError)
			return
		}
		for _, row := range rows {
			tt := timelineTurn{
				Turn: row.Turn, HitTokens: row.HitTokens, MissTokens: row.MissTokens,
				OutputTokens: row.OutputTokens, CostCNY: row.CostCNY,
				Evicted: row.Evicted, Why: row.Why,
			}
			if n := len(epochs); n > 0 && epochs[n-1].EpochID == row.EpochID {
				epochs[n-1].Turns = append(epochs[n-1].Turns, tt)
			} else {
				epochs = append(epochs, timelineEpoch{EpochID: row.EpochID, Turns: []timelineTurn{tt}})
			}
		}
	}
	writeJSON(w, map[string]any{"epochs": epochs})
}
