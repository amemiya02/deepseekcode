package gateway

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/amemiya02/deepseekcode/internal/config"
)

// defaultModels is the fallback model list advertised when config can't be
// loaded (e.g. in tests or before onboarding). The two-model Flash/Pro pair
// is dsc's routing default.
var defaultModels = []string{"deepseek-v4-flash", "deepseek-v4-pro"}

// modelState holds the gateway's current model/effort selection. It is seeded
// from config.Load() at construction and mutated by POST /v1/model and
// /v1/effort. It is the gateway's view; applying it to a live agent run is the
// concern of a later wave (the session factory reads config today).
type modelState struct {
	mu     sync.Mutex
	models []string
	active string
	effort string
}

// newModelState seeds selection from config; on any load error it falls back to
// defaultModels so the endpoints always work.
func newModelState() *modelState {
	ms := &modelState{models: defaultModels, active: defaultModels[0], effort: "medium"}
	cfg, err := config.Load()
	if err != nil {
		return ms
	}
	if cfg.Defaults.Model != "" {
		ms.active = cfg.Defaults.Model
		if !contains(ms.models, ms.active) {
			ms.models = append([]string{ms.active}, ms.models...)
		}
	}
	if cfg.Defaults.ReasoningEffort != "" {
		ms.effort = cfg.Defaults.ReasoningEffort
	}
	return ms
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// handleModels implements GET /v1/models.
func (h *Handler) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.models.mu.Lock()
	resp := map[string]any{"models": h.models.models, "active": h.models.active, "effort": h.models.effort}
	h.models.mu.Unlock()
	writeJSON(w, resp)
}

// handleModel implements POST /v1/model. It accepts only a model in the
// advertised list; an unknown model is 400.
func (h *Handler) handleModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	h.models.mu.Lock()
	ok := contains(h.models.models, body.Model)
	if ok {
		h.models.active = body.Model
	}
	h.models.mu.Unlock()
	if !ok {
		http.Error(w, "unknown model", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleEffort implements POST /v1/effort. Accepts low|medium|high.
func (h *Handler) handleEffort(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Effort string `json:"effort"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	switch body.Effort {
	case "low", "medium", "high":
	default:
		http.Error(w, "invalid effort (low|medium|high)", http.StatusBadRequest)
		return
	}
	h.models.mu.Lock()
	h.models.effort = body.Effort
	h.models.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

// handleBalance implements GET /v1/balance. Wallet balance comes from the
// provider's balance_url; that is not wired in Wave 1, so the gateway reports
// is_available=false (the SPA renders "balance unavailable") rather than
// erroring. A later wave fetches the real balance.
func (h *Handler) handleBalance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{
		"currency": "CNY", "available": 0.0, "is_available": false,
	})
}
