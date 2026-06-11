package gateway

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/amemiya02/deepseekcode/internal/config"
	"github.com/amemiya02/deepseekcode/internal/llm"
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
	mu             sync.Mutex
	models         []string
	descriptors    []modelDescriptor
	active         string
	effort         string
	activeProvider string
	storedClient   *llm.Client
	storedCaps     llm.Capabilities
}

// modelCaps is the per-model capability flags the composer renders as glyphs.
type modelCaps struct {
	Vision    bool `json:"vision"`
	Tools     bool `json:"tools"`
	Reasoning bool `json:"reasoning"`
}

// modelEffort describes a model's effort control. Kind "levels" carries the
// allowed levels + default; "none" tells the composer to hide the effort chip.
type modelEffort struct {
	Kind    string   `json:"kind"` // "levels" | "none"
	Levels  []string `json:"levels,omitempty"`
	Default string   `json:"default,omitempty"`
}

// modelDescriptor is one row of the capability-driven /v1/models contract.
type modelDescriptor struct {
	ID       string      `json:"id"`
	Label    string      `json:"label"`
	Provider string      `json:"provider"`
	Caps     modelCaps   `json:"caps"`
	Effort   modelEffort `json:"effort"`
	Context  int         `json:"context"`
}

// modelLabel maps a bare model id to a friendly display label. SupportsModels
// carries only ids; the picker wants human labels.
func modelLabel(id string) string {
	switch id {
	case "deepseek-v4-flash":
		return "DeepSeek V4 Flash"
	case "deepseek-v4-pro":
		return "DeepSeek V4 Pro"
	default:
		return id
	}
}

// buildDescriptors enumerates the active provider's advertised models into
// capability descriptors. Legacy aliases are filtered (still POST-accepted by
// handleModel for back-compat, just not shown as picker rows). On any provider
// error it returns nil and the caller falls back to the bare model list.
func buildDescriptors(cfg config.Config) []modelDescriptor {
	providerName := cfg.Active.Provider
	if providerName == "" {
		providerName = "deepseek"
	}
	// Use the static ProviderCapabilities helper — it returns capability values
	// without constructing a live provider instance (no credentials, no HTTP
	// client). This ensures that if a future provider's constructor or
	// Capabilities() ever touches the config the build-time descriptor path
	// remains safe and the failure is explicit rather than silent.
	caps, err := llm.ProviderCapabilities(providerName)
	if err != nil {
		return nil
	}
	// The descriptor's effort default is the model's *capability* default — a
	// stable property of the model, not the user's currently-selected effort
	// (that lives in the response's top-level "effort"). It is read exclusively
	// from config.Default() (the provider/model baseline, "max" for DeepSeek V4)
	// so the user's loaded preference never leaks into this capability field.
	def := config.Default().Defaults.ReasoningEffort
	if def == "" {
		def = "max"
	}
	var effort modelEffort
	if len(caps.ReasoningEfforts) > 0 {
		levels := make([]string, len(caps.ReasoningEfforts))
		for i, e := range caps.ReasoningEfforts {
			levels[i] = string(e) // type ReasoningEffort string → "low"/"medium"/"high"/"max"
		}
		effort = modelEffort{Kind: "levels", Levels: levels, Default: def}
	} else {
		effort = modelEffort{Kind: "none"}
	}
	var out []modelDescriptor
	for _, id := range caps.SupportsModels {
		if config.IsLegacyDeepSeekAlias(id) {
			continue
		}
		// All DeepSeek V4 models (Flash and Pro) share the same effort tiers and
		// context window today, so every descriptor reuses the provider-level
		// `effort` value and `caps.MaxContextTokens`. The descriptor shape is
		// per-model on purpose — revisit this loop and compute these per id if a
		// future model diverges (e.g. a model without reasoning, or a different
		// context window), otherwise it would silently advertise a wrong descriptor.
		out = append(out, modelDescriptor{
			ID:       id,
			Label:    modelLabel(id),
			Provider: providerName,
			Caps:     modelCaps{Vision: false, Tools: true, Reasoning: caps.Thinking},
			Effort:   effort,
			Context:  caps.MaxContextTokens,
		})
	}
	return out
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
		if !slices.Contains(ms.models, ms.active) {
			ms.models = append([]string{ms.active}, ms.models...)
		}
	}
	if cfg.Defaults.ReasoningEffort != "" {
		ms.effort = cfg.Defaults.ReasoningEffort
	}
	// TODO(p3.4): descriptors are built once here at construction and cached on
	// modelState with no invalidation. If the user later switches provider/model
	// in settings, GET /v1/models keeps advertising the descriptors captured at
	// boot until the process restarts. Rebuild (or invalidate) descriptors when
	// the active provider config changes once settings can mutate it live.
	if d := buildDescriptors(cfg); len(d) > 0 {
		ms.descriptors = d
		// Keep ms.models in sync with the advertised descriptor IDs so a model
		// shown by GET /v1/models is always accepted by POST /v1/model (which
		// validates against ms.models). Without this, a descriptor row absent
		// from the bare config list would 400 on selection.
		for _, dd := range d {
			if !slices.Contains(ms.models, dd.ID) {
				ms.models = append(ms.models, dd.ID)
			}
		}
	}
	return ms
}

// handleModels implements GET /v1/models.
func (h *Handler) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.models.mu.Lock()
	resp := map[string]any{"active": h.models.active, "effort": h.models.effort}
	h.models.mu.Unlock()

	if h.reg != nil {
		rows, _ := h.reg.List(r.Context())
		def := config.Default().Defaults.ReasoningEffort
		if def == "" {
			def = "max"
		}
		descs := make([]map[string]any, 0, len(rows))
		for _, m := range rows {
			var effort map[string]any
			if len(m.Caps.ReasoningEfforts) > 0 {
				levels := make([]string, len(m.Caps.ReasoningEfforts))
				for i, e := range m.Caps.ReasoningEfforts {
					levels[i] = string(e)
				}
				effort = map[string]any{"kind": "levels", "levels": levels, "default": def}
			} else {
				effort = map[string]any{"kind": "none"}
			}
			descs = append(descs, map[string]any{
				"id": m.ID, "label": modelLabel(m.ID), "provider": m.Provider,
				"available": m.Available,
				"caps":      map[string]bool{"vision": false, "tools": true, "reasoning": m.Caps.Thinking},
				"effort":    effort,
				"context":   m.Caps.MaxContextTokens,
			})
		}
		resp["models"] = descs
	} else if len(h.models.descriptors) > 0 {
		resp["models"] = h.models.descriptors
	} else {
		resp["models"] = h.models.models
	}
	writeJSON(w, resp)
}

// handleModel implements POST /v1/model. It accepts only a model in the
// advertised list; an unknown model is 400. When a registry is available it
// also accepts a provider field and delegates to modelreg.Switch.
func (h *Handler) handleModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Model    string `json:"model"`
		Provider string `json:"provider"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if h.reg != nil {
		res, err := h.reg.Switch(r.Context(), body.Provider, body.Model)
		if err == nil {
			h.models.mu.Lock()
			h.models.active = res.Model
			h.models.effort = res.Effort
			h.models.activeProvider = res.Provider
			h.models.storedClient = res.Client
			h.models.storedCaps = res.Caps
			h.models.mu.Unlock()
			w.WriteHeader(http.StatusOK)
			return
		}
		// Registry Switch failed (e.g. default provider has no key in the
		// merged config). Fall through to the legacy model-list path.
	}

	h.models.mu.Lock()
	ok := slices.Contains(h.models.models, body.Model)
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

// handleEffort implements POST /v1/effort. Accepts low|medium|high|max
// (max is the configured DeepSeek V4 default).
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
	h.models.mu.Lock()
	levels := h.models.validEffortLevels()
	ok := slices.Contains(levels, body.Effort)
	if ok {
		h.models.effort = body.Effort
	}
	h.models.mu.Unlock()
	if !ok {
		http.Error(w, "invalid effort ("+strings.Join(levels, "|")+")", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// fallbackEffortLevels is the DeepSeek V4 baseline effort set used when the
// active model has no capability descriptor (e.g. a config-less boot). It mirrors
// llm.DeepSeekProvider.Capabilities().ReasoningEfforts. Declared as an array so
// validEffortLevels() always returns a fresh slice — callers that append cannot
// mutate this package-level value.
var fallbackEffortLevels = [4]string{"low", "medium", "high", "max"}

// validEffortLevels returns the effort levels POST /v1/effort accepts, derived
// from the active model's capability descriptor (descriptor.Effort.Levels) so the
// validator stays in sync automatically when a provider or model changes the
// valid set. Falls back to the DeepSeek V4 baseline when no descriptor matches
// the active model (descriptor-less boot or a kind=="none" effort control).
// Always returns a fresh copy so callers may not mutate the source.
// Caller must hold ms.mu.
func (ms *modelState) validEffortLevels() []string {
	for _, d := range ms.descriptors {
		if d.ID == ms.active && d.Effort.Kind == "levels" && len(d.Effort.Levels) > 0 {
			return append([]string(nil), d.Effort.Levels...)
		}
	}
	return fallbackEffortLevels[:]
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
