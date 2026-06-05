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
	Turns     int    `json:"turns"`
	CreatedAt int64  `json:"created_at"` // epoch ms — matches the SPA Session contract
	UpdatedAt int64  `json:"updated_at"` // epoch ms
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
	if !ok {
		return nil, false
	}
	// Return a copy so callers cannot race-mutate the stored pointer.
	cp := *m
	return &cp, true
}

// update applies fn to a copy of the stored sessionMeta and writes the result
// back under the lock, preventing data races on concurrent PATCH requests.
// Returns (updated copy, true) when the id exists, (nil, false) otherwise.
func (s *sessionStore) update(id string, fn func(*sessionMeta)) (*sessionMeta, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.meta[id]
	if !ok {
		return nil, false
	}
	cp := *m
	fn(&cp)
	s.meta[id] = &cp
	return &cp, true
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
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out
}

// deriveTitle makes a short, single-line session title from the first prompt.
// It trims whitespace, takes the first line, and caps length at ~46 runes on a
// word boundary so the rail stays tidy. Empty input yields "New session".
func deriveTitle(prompt string) string {
	s := strings.TrimSpace(prompt)
	if s == "" {
		return "New session"
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	const max = 46
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	// Only word-truncate when the cut falls mid-word (next rune is not a space).
	r := runes[:max]
	if runes[max] != ' ' {
		if j := strings.LastIndexByte(string(r), ' '); j > 0 {
			return strings.TrimRight(string(r[:j]), " ")
		}
	}
	return string(r)
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
		now := time.Now().UnixMilli()
		m := &sessionMeta{ID: id, Title: body.Title, CreatedAt: now, UpdatedAt: now}
		if m.Title == "" {
			m.Title = id
		}
		h.sessions.put(m)
		writeJSON(w, m)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSessionByID implements GET/PATCH/DELETE /v1/sessions/{id} and
// dispatches sub-resource requests (e.g. /v1/sessions/{id}/timeline).
func (h *Handler) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/v1/sessions/")
	// Dispatch sub-resources before the plain-id validation so that paths like
	// "someID/timeline" reach their handler without a 400.
	if id, suffix, ok := strings.Cut(rest, "/"); ok {
		switch suffix {
		case "timeline":
			_ = id // accepted but not yet used for per-session filtering
			h.handleSessionTimeline(w, r)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
		return
	}
	id := rest
	if id == "" {
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
			"id": m.ID, "title": m.Title, "turns": m.Turns,
			"created_at": m.CreatedAt, "updated_at": m.UpdatedAt, "messages": []any{},
		})
	case http.MethodPatch:
		var body struct {
			Title string `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		m, ok := h.sessions.update(id, func(m *sessionMeta) {
			if body.Title != "" {
				m.Title = body.Title
			}
			m.UpdatedAt = time.Now().UnixMilli()
		})
		if !ok {
			http.Error(w, "session not found", http.StatusNotFound)
			return
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
