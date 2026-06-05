package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/amemiya02/deepseekcode/internal/acp"
)

// steerRequest is the POST /v1/steer body.
type steerRequest struct {
	SessionID string `json:"session_id"`
	Prompt    string `json:"prompt"`
}

// handleSteer implements POST /v1/steer. It injects a mid-turn instruction into
// the session's in-flight run via the SessionManager. Steering an unknown
// session is a no-op 200 (idempotent), matching handleCancel.
func (h *Handler) handleSteer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req steerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	h.sm.Steer(req.SessionID, req.Prompt)
	w.WriteHeader(http.StatusOK)
}

// cancelRequest is the POST /v1/cancel body.
type cancelRequest struct {
	SessionID string `json:"session_id"`
}

// handleCancel implements POST /v1/cancel. It cancels the session's run via the
// SessionManager. Cancelling an unknown session is a no-op 200 (idempotent).
func (h *Handler) handleCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req cancelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	h.sm.Cancel(req.SessionID)
	w.WriteHeader(http.StatusOK)
}

// permissionRequest is the POST /v1/permission body.
type permissionRequest struct {
	ID       string `json:"id"`
	Decision string `json:"decision"` // deny | once | session | always
}

// handlePermission implements POST /v1/permission. It looks up the pending
// permission by id, resolves it with the mapped decision, and removes it.
func (h *Handler) handlePermission(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req permissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	h.mu.Lock()
	p, ok := h.pendingPerm[req.ID]
	if ok {
		delete(h.pendingPerm, req.ID)
	}
	h.mu.Unlock()
	if !ok {
		http.Error(w, "unknown permission id", http.StatusNotFound)
		return
	}
	p.respond(decodePermissionDecision(req.Decision))
	w.WriteHeader(http.StatusOK)
}

// decodePermissionDecision maps the wire string to an acp.PermissionDecision.
// An unrecognized value is treated as deny (fail-closed).
func decodePermissionDecision(s string) acp.PermissionDecision {
	switch s {
	case "once":
		return acp.PermissionAllowOnce
	case "session":
		return acp.PermissionAllowSession
	case "always":
		return acp.PermissionAllowAlways
	default:
		return acp.PermissionDeny
	}
}

// answerRequest is the POST /v1/answer body. Answers is per-question selected
// labels; Text is a free-text answer (mapped to a single one-element row);
// Chat=true means "just chat" (no selection — an empty answer set).
type answerRequest struct {
	ID      string     `json:"id"`
	Answers [][]string `json:"answers,omitempty"`
	Text    string     `json:"text,omitempty"`
	Chat    bool       `json:"chat,omitempty"`
}

// handleAnswer implements POST /v1/answer. It resolves the pending ask by id.
func (h *Handler) handleAnswer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req answerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	h.mu.Lock()
	a, ok := h.pendingAsk[req.ID]
	if ok {
		delete(h.pendingAsk, req.ID)
	}
	h.mu.Unlock()
	if !ok {
		http.Error(w, "unknown ask id", http.StatusNotFound)
		return
	}
	answers := req.Answers
	if answers == nil && req.Text != "" {
		answers = [][]string{{req.Text}}
	}
	a.answer(answers) // Chat / empty → nil answers (the agent sees "no selection")
	w.WriteHeader(http.StatusOK)
}
