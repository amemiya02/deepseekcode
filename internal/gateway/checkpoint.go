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

// branchRequest forks a session at an explicit branch_point (message index).
// Messages are NOT copied: Store.Replay walks parents at read time, so the new
// child shares history up to branch_point and diverges after it.
type branchRequest struct {
	SessionID   string `json:"session_id"`
	BranchPoint int    `json:"branch_point"`
}

type branchResponse struct {
	SessionID   string `json:"session_id"`
	ParentID    string `json:"parent_id"`
	BranchPoint int    `json:"branch_point"`
}

func (h *Handler) handleBranch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.store == nil {
		http.Error(w, "branch unavailable: no session store", http.StatusNotImplemented)
		return
	}
	var req branchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.SessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}
	child, err := h.store.NewBranch(r.Context(), req.SessionID, req.BranchPoint)
	if err != nil {
		http.Error(w, "branch: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(branchResponse{
		SessionID:   child.ID,
		ParentID:    child.ParentID,
		BranchPoint: child.BranchPoint,
	})
}

// forkRequest forks at the END of the current history (branch_point = message
// count) — the common "continue from here on a clean copy" gesture. It is
// branch with an implicit branch_point, kept distinct so the SPA can label the
// two intents differently.
type forkRequest struct {
	SessionID string `json:"session_id"`
}

func (h *Handler) handleFork(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.store == nil {
		http.Error(w, "fork unavailable: no session store", http.StatusNotImplemented)
		return
	}
	var req forkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.SessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}
	count, err := h.store.CountMessages(r.Context(), req.SessionID)
	if err != nil {
		http.Error(w, "fork: "+err.Error(), http.StatusInternalServerError)
		return
	}
	child, err := h.store.NewBranch(r.Context(), req.SessionID, count)
	if err != nil {
		http.Error(w, "fork: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(branchResponse{
		SessionID:   child.ID,
		ParentID:    child.ParentID,
		BranchPoint: child.BranchPoint,
	})
}

// switchRequest selects an existing session and returns its replayed transcript
// so the SPA can repaint the conversation when the user clicks a sidebar item or
// after a branch/fork. The "active session" is a client-side notion (the gateway
// is stateless per-request); switch just touches last_used and replays.
type switchRequest struct {
	SessionID string `json:"session_id"`
}

// transcriptMessage is the SPA-facing flattened message shape.
type transcriptMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type switchResponse struct {
	SessionID string              `json:"session_id"`
	Messages  []transcriptMessage `json:"messages"`
}

func (h *Handler) handleSwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.store == nil {
		http.Error(w, "switch unavailable: no session store", http.StatusNotImplemented)
		return
	}
	var req switchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.SessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}
	msgs, err := h.store.Replay(r.Context(), req.SessionID)
	if err != nil {
		http.Error(w, "switch: "+err.Error(), http.StatusNotFound)
		return
	}
	_ = h.store.TouchLastUsed(r.Context(), req.SessionID)

	out := switchResponse{SessionID: req.SessionID}
	for _, m := range msgs {
		out.Messages = append(out.Messages, transcriptMessage{Role: m.Role, Text: m.Content})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// summarizeRequest collapses a message range into a single synthetic summary
// row via Store.ReplaceWithCompaction (the same primitive auto-compaction uses).
//   - mode "upto": collapse [0, index).
//   - mode "from": collapse [index, end).
// "summary" is the replacement text; when empty a neutral placeholder is used so
// the row is never blank (the SPA shows it as a folded "summarized" marker).
type summarizeRequest struct {
	SessionID string `json:"session_id"`
	Mode      string `json:"mode"`
	Index     int    `json:"index"`
	Summary   string `json:"summary"`
}

type summarizeResponse struct {
	SummaryIdx int `json:"summary_idx"`
}

func (h *Handler) handleSummarize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.store == nil {
		http.Error(w, "summarize unavailable: no session store", http.StatusNotImplemented)
		return
	}
	var req summarizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.SessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}
	summary := req.Summary
	if summary == "" {
		summary = "(summarized)"
	}
	count, err := h.store.CountMessages(r.Context(), req.SessionID)
	if err != nil {
		http.Error(w, "summarize: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var fromIdx, toIdx int
	switch req.Mode {
	case "from":
		fromIdx, toIdx = req.Index, count
	default: // "upto" (and empty) collapse the prefix.
		fromIdx, toIdx = 0, req.Index
	}
	idx, err := h.store.ReplaceWithCompaction(r.Context(), req.SessionID, fromIdx, toIdx, summary)
	if err != nil {
		http.Error(w, "summarize: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(summarizeResponse{SummaryIdx: idx})
}
