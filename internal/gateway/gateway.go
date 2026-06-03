// Package gateway exposes the web SPA's /v1 HTTP+SSE API and serves the
// embedded SPA itself. It is the single handler that makes the Svelte web app
// (web/) and the Wails desktop wrapper (desktop/) function end-to-end against
// the agent.
//
// Routes (the contract is defined by web/src/lib/api.ts; the server matches
// the SPA, not the other way around):
//
//	POST /v1/prompt   {"prompt", "session_id"?} -> {"request_id","session_id"}
//	GET  /v1/cache                              -> CacheReport JSON
//	GET  /v1/events?session_id=X                -> text/event-stream SSE
//	GET  /*                                     -> embedded SPA (webapp.Handler)
//
// The /v1 routes reuse the acp SessionManager + AgentFactory machinery; this
// package does not reimplement agent streaming. It maps acp.AgentEvent values
// onto the SPA's named SSE events (delta/tool/step/done).
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/amemiya02/deepseekcode/internal/acp"
	"github.com/amemiya02/deepseekcode/webapp"
)

// Handler is the gateway's composite http.Handler: the three /v1 routes plus a
// catch-all that serves the embedded SPA.
type Handler struct {
	mux       *http.ServeMux
	sm        *acp.SessionManager
	tracePath string
	hub       *hub
	reqSeq    atomic.Int64
}

// NewHandler builds the gateway handler over an existing SessionManager. The
// SessionManager supplies the AgentFactory (real in production, a stub in
// tests). tracePath is read on demand by /v1/cache; an absent file yields a
// zero-valued report rather than an error.
func NewHandler(sm *acp.SessionManager, tracePath string) http.Handler {
	h := &Handler{
		mux:       http.NewServeMux(),
		sm:        sm,
		tracePath: tracePath,
		hub:       newHub(),
	}
	h.mux.HandleFunc("/v1/prompt", h.handlePrompt)
	h.mux.HandleFunc("/v1/cache", h.handleCache)
	h.mux.HandleFunc("/v1/events", h.handleEvents)
	// Catch-all: anything not under /v1 is served by the embedded SPA. The
	// "/" pattern is the lowest-priority match in a ServeMux, so the explicit
	// /v1/* patterns above always win.
	h.mux.Handle("/", webapp.Handler())
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

// handleCache implements GET /v1/cache. It returns the CacheReport derived from
// the configured trace path. A missing trace yields a zero-valued report (200),
// never a 500.
func (h *Handler) handleCache(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	report, err := buildCacheReport(h.tracePath)
	if err != nil {
		http.Error(w, "cache report: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(report); err != nil {
		// Headers already sent; nothing further to do.
		_ = err
	}
}

// promptRequest is the POST /v1/prompt request body.
type promptRequest struct {
	Prompt    string `json:"prompt"`
	SessionID string `json:"session_id,omitempty"`
}

// promptResponse is the POST /v1/prompt response body.
type promptResponse struct {
	RequestID string `json:"request_id"`
	SessionID string `json:"session_id"`
}

// handlePrompt implements POST /v1/prompt. It creates a session (or reuses the
// one named by session_id), kicks off the agent run in the background, and
// returns request_id + session_id immediately. Streaming is observed via
// GET /v1/events for the returned session_id.
func (h *Handler) handlePrompt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req promptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.Prompt == "" {
		http.Error(w, "prompt is required", http.StatusBadRequest)
		return
	}

	// Resolve the session: reuse an existing one if session_id was supplied and
	// still exists, otherwise create a fresh session.
	sessionID := req.SessionID
	if sessionID == "" || !h.sm.Has(sessionID) {
		id, err := h.sm.NewSession(r.Context(), "")
		if err != nil {
			http.Error(w, "create session: "+err.Error(), http.StatusInternalServerError)
			return
		}
		sessionID = id
	}

	// The session context (not the request context) drives the run so that the
	// agent keeps running after this POST returns and its connection closes.
	if _, err := h.sm.Lookup(sessionID); err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	runCtx := h.sm.SessionCtx(sessionID)

	requestID := fmt.Sprintf("req-%d", h.reqSeq.Add(1))
	prompt := req.Prompt
	sid := sessionID

	go func() {
		onEvent := func(ev acp.AgentEvent) {
			h.hub.broadcast(sid, mapAgentEvent(ev))
		}
		if err := h.sm.Prompt(runCtx, sid, prompt, onEvent); err != nil {
			// Ensure subscribers see a terminal "done" even on dispatch failure
			// (e.g. session vanished) so the SPA's onDone fires.
			h.hub.broadcast(sid, sseEvent{name: "done", data: "error: " + err.Error()})
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(promptResponse{RequestID: requestID, SessionID: sessionID}); err != nil {
		_ = err
	}
}

// handleEvents implements GET /v1/events?session_id=X as an SSE stream of the
// named events the SPA listens for: delta, tool, step, done.
func (h *Handler) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	sub, unsub := h.hub.subscribe(sessionID, r.Context())
	defer unsub()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-sub.ch:
			// SSE framing: event: <name>\ndata: <payload>\n\n
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.name, ev.data)
			flusher.Flush()
			if ev.name == "done" {
				return
			}
		}
	}
}

// Start builds a default gateway (real agent factory, default trace path) and
// runs it on 127.0.0.1:port until ctx is cancelled. It is what the desktop
// wrapper calls in-process; it always binds loopback so the in-process gateway
// is never exposed to the network.
func Start(ctx context.Context, port int) error {
	sm := acp.NewSessionManager(acp.RealAgentFactory)
	handler := NewHandler(sm, defaultTracePath())

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	server := &http.Server{Addr: addr, Handler: handler}

	errCh := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}
