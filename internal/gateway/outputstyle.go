package gateway

import (
	"encoding/json"
	"net/http"
	"sync"
)

// knownOutputStyles is the accepted style set (spec §6). default = balanced.
var knownOutputStyles = map[string]bool{
	"default": true, "concise": true, "explanatory": true, "learning": true,
}

// outputStyleState holds the gateway's current output-style selection per
// session. Applying it to a live run is a later-wave concern; Wave 1 records it.
type outputStyleState struct {
	mu    sync.Mutex
	style map[string]string // session_id -> style
}

func newOutputStyleState() *outputStyleState {
	return &outputStyleState{style: make(map[string]string)}
}

// handleOutputStyle implements POST /v1/output-style {session_id, style}.
func (h *Handler) handleOutputStyle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		SessionID string `json:"session_id"`
		Style     string `json:"style"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if !knownOutputStyles[body.Style] {
		http.Error(w, "unknown output style", http.StatusBadRequest)
		return
	}
	h.outputStyle.mu.Lock()
	h.outputStyle.style[body.SessionID] = body.Style
	h.outputStyle.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}
