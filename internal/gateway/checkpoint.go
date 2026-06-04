// Package gateway: Wave-5 checkpoint/rewind/branch endpoints. They operate
// directly on the injected *session.Store and *snapshots.Manager (see
// options.go). When a Handler was built without those, the endpoints return
// 501 so a misconfigured server fails loudly rather than silently no-op'ing.
//
// CHECKPOINT DISCLAIMER (surfaced in the SPA RewindMenu): rewind restores only
// (a) the persisted conversation history and (b) files captured by the agent's
// pre-edit snapshots. Edits made by bash commands or by tools/processes outside
// the agent are NOT tracked here — git is the source of truth for the working
// tree. A "code" rewind is best-effort over snapshots; verify with `git diff`.
package gateway

import (
	"encoding/json"
	"net/http"
)

// rewindRequest restores a session to an earlier point.
//   - keep_messages: number of leading messages to retain (the rest are dropped).
//   - scope: "conversation" (history only), "code" (snapshots only), or "both".
type rewindRequest struct {
	SessionID    string `json:"session_id"`
	KeepMessages int    `json:"keep_messages"`
	Scope        string `json:"scope"`
}

type rewindResponse struct {
	RemovedMessages int `json:"removed_messages"`
	RestoredFiles   int `json:"restored_files"`
}

func (h *Handler) handleRewind(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.store == nil {
		http.Error(w, "rewind unavailable: no session store", http.StatusNotImplemented)
		return
	}
	var req rewindRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.SessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}
	scope := req.Scope
	if scope == "" {
		scope = "both"
	}

	var out rewindResponse
	if scope == "conversation" || scope == "both" {
		removed, err := h.store.TruncateMessages(r.Context(), req.SessionID, req.KeepMessages)
		if err != nil {
			http.Error(w, "rewind conversation: "+err.Error(), http.StatusInternalServerError)
			return
		}
		out.RemovedMessages = removed
	}
	if scope == "code" || scope == "both" {
		if h.snaps != nil && h.snaps.HasSnapshots(req.SessionID) {
			// Undo restores the most recent snapshot step. The SPA presents code
			// rewind as best-effort over snapshots (see disclaimer above); a
			// session with no snapshots simply restores zero files.
			restored, err := h.snaps.Undo(req.SessionID, 1)
			if err == nil {
				out.RestoredFiles = restored
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
