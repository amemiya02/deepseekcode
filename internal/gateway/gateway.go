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
	"sync"
	"sync/atomic"
	"time"

	"github.com/amemiya02/deepseekcode/internal/acp"
	"github.com/amemiya02/deepseekcode/webapp"
)

// pendingPermission is an in-flight permission awaiting POST /v1/permission.
type pendingPermission struct {
	respond func(acp.PermissionDecision)
}

// pendingAsk is an in-flight question awaiting POST /v1/answer.
type pendingAsk struct {
	answer func(answers [][]string)
}

// permissionOption is one SPA-facing permission choice. value is the wire
// string POSTed back to /v1/permission (deny|once|session|always); label and
// description are the human-readable UI text (the SPA routes them through t()).
type permissionOption struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// permissionOptions returns the canonical 4-tier permission choices (spec §7.B).
func permissionOptions() []permissionOption {
	return []permissionOption{
		{Value: "deny", Label: "Deny", Description: "Reject this tool call"},
		{Value: "once", Label: "Allow once", Description: "Permit this one call"},
		{Value: "session", Label: "Allow for session", Description: "Permit this tool for the rest of this session"},
		{Value: "always", Label: "Always allow", Description: "Permit this tool from now on"},
	}
}

// Handler is the gateway's composite http.Handler: the three /v1 routes plus a
// catch-all that serves the embedded SPA.
type Handler struct {
	mux         *http.ServeMux
	sm          *acp.SessionManager
	tracePath   string
	root        string // workspace root for /v1/files, /v1/file, /v1/changed
	hub         *hub
	reqSeq      atomic.Int64
	mu          sync.Mutex
	pendingPerm map[string]pendingPermission
	pendingAsk  map[string]pendingAsk
	intSeq      atomic.Int64 // interaction id sequence
	sessions    *sessionStore
	models      *modelState
	outputStyle *outputStyleState
}

// NewHandler builds the gateway handler over an existing SessionManager. The
// SessionManager supplies the AgentFactory (real in production, a stub in
// tests). tracePath is read on demand by /v1/cache; an absent file yields a
// zero-valued report rather than an error.
func NewHandler(sm *acp.SessionManager, tracePath string) http.Handler {
	return NewHandlerWithRoot(sm, tracePath, "")
}

// NewHandlerWithRoot is like NewHandler but also sets the workspace root used
// by the /v1/files, /v1/file and /v1/changed endpoints. When root is empty
// those endpoints return "no workspace root" (400). In production, Start passes
// the process working directory; tests pass a hermetic temp dir.
func NewHandlerWithRoot(sm *acp.SessionManager, tracePath, root string) http.Handler {
	h := &Handler{
		mux:         http.NewServeMux(),
		sm:          sm,
		tracePath:   tracePath,
		root:        root,
		hub:         newHub(),
		pendingPerm: make(map[string]pendingPermission),
		pendingAsk:  make(map[string]pendingAsk),
		sessions:    newSessionStore(),
		models:      newModelState(),
		outputStyle: newOutputStyleState(),
	}
	h.mux.HandleFunc("/v1/prompt", h.handlePrompt)
	h.mux.HandleFunc("/v1/cache", h.handleCache)
	h.mux.HandleFunc("/v1/events", h.handleEvents)
	h.mux.HandleFunc("/v1/cancel", h.handleCancel)
	h.mux.HandleFunc("/v1/permission", h.handlePermission)
	h.mux.HandleFunc("/v1/answer", h.handleAnswer)
	h.mux.HandleFunc("/v1/sessions", h.handleSessions)
	h.mux.HandleFunc("/v1/sessions/", h.handleSessionByID)
	h.mux.HandleFunc("/v1/cache/ledger", h.handleCacheLedger)
	h.mux.HandleFunc("/v1/models", h.handleModels)
	h.mux.HandleFunc("/v1/model", h.handleModel)
	h.mux.HandleFunc("/v1/effort", h.handleEffort)
	h.mux.HandleFunc("/v1/balance", h.handleBalance)
	h.mux.HandleFunc("/v1/output-style", h.handleOutputStyle)
	h.mux.HandleFunc("/v1/files", h.handleFiles)
	h.mux.HandleFunc("/v1/file", h.handleFile)
	h.mux.HandleFunc("/v1/changed", h.handleChanged)
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
		// Per-run live-signal accumulators (one prompt = one goroutine).
		var (
			cacheSum, cacheN float64 // rolling avg of turn_pct
			sessionCNY       float64
			runningJobs      int
			prefixCount      int
		)
		onEvent := func(ev acp.AgentEvent) {
			switch ev.Kind {
			case acp.EventKindPermission:
				id := fmt.Sprintf("perm-%d", h.intSeq.Add(1))
				h.mu.Lock()
				h.pendingPerm[id] = pendingPermission{respond: ev.Respond}
				h.mu.Unlock()
				// ev.ToolArgs is a raw JSON string; unmarshal into RawMessage so
				// the SSE payload carries args as a JSON object (Record<string,unknown>),
				// not a double-encoded JSON string — Contract 2.
				var argsRaw json.RawMessage
				if err := json.Unmarshal([]byte(ev.ToolArgs), &argsRaw); err != nil {
					// Fallback: wrap as a single-field object so the wire type stays object.
					argsRaw = json.RawMessage(`{"_raw":` + mustJSON(ev.ToolArgs) + `}`)
				}
				h.hub.broadcast(sid, sseEvent{name: "permission_request", data: mustJSON(map[string]any{
					"id": id, "tool": ev.ToolName, "args": argsRaw,
					"options": permissionOptions(),
				})})
			case acp.EventKindAsk:
				id := fmt.Sprintf("ask-%d", h.intSeq.Add(1))
				h.mu.Lock()
				h.pendingAsk[id] = pendingAsk{answer: ev.Answer}
				h.mu.Unlock()
				h.hub.broadcast(sid, sseEvent{name: "ask_request", data: mustJSON(map[string]any{
					"id": id, "questions": ev.Questions,
				})})
			case acp.EventKindCache:
				if ev.TurnPct > 0 || !ev.Eviction { // eviction-only frames don't move the avg
					cacheSum += ev.TurnPct
					cacheN++
				}
				if !ev.Eviction {
					prefixCount++ // one freshly-priced turn ≈ one active prefix epoch (approx)
				}
				ev.AvgPct = 0
				if cacheN > 0 {
					ev.AvgPct = cacheSum / cacheN
				}
				ev.Prefixes = prefixCount
				h.hub.broadcast(sid, mapAgentEvent(ev))
			case acp.EventKindCost:
				sessionCNY += ev.TurnCNY
				ev.SessionCNY = sessionCNY
				h.hub.broadcast(sid, mapAgentEvent(ev))
			case acp.EventKindJob:
				runningJobs += ev.Running // ev.Running is a +1/−1 delta from the adapter
				if runningJobs < 0 {
					runningJobs = 0
				}
				ev.Running = runningJobs
				h.hub.broadcast(sid, mapAgentEvent(ev))
			default:
				h.hub.broadcast(sid, mapAgentEvent(ev))
			}
		}
		if err := h.sm.Prompt(runCtx, sid, prompt, onEvent); err != nil {
			// Ensure subscribers see a terminal "turn_done" even on dispatch failure
			// (e.g. session vanished) so the SPA's onDone fires.
			h.hub.broadcast(sid, sseEvent{name: "turn_done", data: mustJSON(map[string]any{
				"stop_reason": "error: " + err.Error(),
			})})
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
			if ev.name == "turn_done" {
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
	handler = withAuth(loadGatewayToken(), handler)

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
