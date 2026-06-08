package gateway

import (
	"net/http"

	"github.com/amemiya02/deepseekcode/internal/version"
)

// activeCaps returns the capability flags for the currently-selected model as a
// flat string->bool map, the shape the web Capabilities view consumes. Falls
// back to an empty map when no descriptor matches the active id (the bare-id
// provider fallback path, where no descriptors are built).
func (h *Handler) activeCaps() map[string]bool {
	h.models.mu.Lock()
	defer h.models.mu.Unlock()
	for _, d := range h.models.descriptors {
		if d.ID == h.models.active {
			return map[string]bool{
				"vision":    d.Caps.Vision,
				"tools":     d.Caps.Tools,
				"reasoning": d.Caps.Reasoning,
			}
		}
	}
	return map[string]bool{}
}

// handleCapabilities serves GET /v1/capabilities — the active model's
// capability flags, consumed by the settings Capabilities view
// (web: fetchCapabilities -> { caps }).
func (h *Handler) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{"caps": h.activeCaps()})
}

// handleRuntime serves GET /v1/runtime — active model, build version, and the
// active model's capability flags, consumed by the RuntimeBanner
// (web: fetchRuntimeInfo -> { model, version, caps }).
func (h *Handler) handleRuntime(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.models.mu.Lock()
	active := h.models.active
	h.models.mu.Unlock()
	// activeCaps takes the same lock; acquire it separately to avoid re-entrancy.
	writeJSON(w, map[string]any{
		"model":   active,
		"version": version.Version,
		"caps":    h.activeCaps(),
	})
}
