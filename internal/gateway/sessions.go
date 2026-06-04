package gateway

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// sessionMeta is the gateway's lightweight session record. It is metadata the
// SPA's session rail needs; the acp.SessionManager owns the live agent state.
// Messages is a placeholder for the persisted transcript (wired to the real
// store in a later wave); for now it is always a non-nil (possibly empty) slice
// so the SPA can render an empty conversation without a null check.
type sessionMeta struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"created_at"`
	TurnCount int    `json:"turn_count"`
}

// sessionStore is an in-memory metadata store keyed by session id. It is
// concurrency-safe. It does NOT replace the acp.SessionManager — it annotates
// sessions with UI-facing metadata (title, creation time, turn count).
type sessionStore struct {
	mu   sync.Mutex
	meta map[string]*sessionMeta
}

func newSessionStore() *sessionStore {
	return &sessionStore{meta: make(map[string]*sessionMeta)}
}

func (s *sessionStore) put(m *sessionMeta) {
	s.mu.Lock()
	s.meta[m.ID] = m
	s.mu.Unlock()
}

func (s *sessionStore) get(id string) (*sessionMeta, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.meta[id]
	return m, ok
}

func (s *sessionStore) delete(id string) {
	s.mu.Lock()
	delete(s.meta, id)
	s.mu.Unlock()
}

func (s *sessionStore) list() []sessionMeta {
	s.mu.Lock()
	out := make([]sessionMeta, 0, len(s.meta))
	for _, m := range s.meta {
		out = append(out, *m)
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

// handleSessions implements GET /v1/sessions (list) and POST /v1/sessions (create).
func (h *Handler) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]any{"sessions": h.sessions.list()})
	case http.MethodPost:
		var body struct {
			Title string `json:"title"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body) // empty body is allowed
		id, err := h.sm.NewSession(r.Context(), "")
		if err != nil {
			http.Error(w, "create session: "+err.Error(), http.StatusInternalServerError)
			return
		}
		m := &sessionMeta{ID: id, Title: body.Title, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
		if m.Title == "" {
			m.Title = id
		}
		h.sessions.put(m)
		writeJSON(w, m)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSessionByID implements GET/PATCH/DELETE /v1/sessions/{id}.
func (h *Handler) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/v1/sessions/")
	if id == "" || strings.Contains(id, "/") {
		http.Error(w, "bad session id", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		m, ok := h.sessions.get(id)
		if !ok {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]any{
			"id": m.ID, "title": m.Title, "created_at": m.CreatedAt,
			"turn_count": m.TurnCount, "messages": []any{},
		})
	case http.MethodPatch:
		m, ok := h.sessions.get(id)
		if !ok {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		var body struct {
			Title string `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if body.Title != "" {
			m.Title = body.Title
			h.sessions.put(m)
		}
		writeJSON(w, m)
	case http.MethodDelete:
		if _, ok := h.sessions.get(id); !ok {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		h.sessions.delete(id)
		h.sm.Cancel(id)
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// writeJSON encodes v as the JSON body with the application/json content type.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
