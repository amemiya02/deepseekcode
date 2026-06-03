package acp

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// sseClient is a pending SSE subscriber for a session.
// done is closed by the stream goroutine when it exits; it is never closed by
// external callers, which eliminates the send-on-closed-channel race.
type sseClient struct {
	ch     chan string
	cancel context.CancelFunc
	ctx    context.Context // associated stream context; Done() == ch-closed signal
}

// sessionStreamState tracks whether an SSE subscriber has registered for a
// session. handlePrompt waits for streamReady before launching the agent
// goroutine so that broadcast never races with an empty subscriber list.
type sessionStreamState struct {
	readyCh chan struct{} // closed once the first SSE subscriber has registered
	once    sync.Once
}

// HTTPGateway is an http.Handler exposing ACP over HTTP+SSE.
//
// Routes:
//
//	POST   /session                       → create session (JSON body: SessionNewParams)
//	POST   /session/{id}/prompt           → send prompt (JSON body: {prompt})
//	GET    /session/{id}/stream           → SSE event stream
//	DELETE /session/{id}                  → cancel session
type HTTPGateway struct {
	sm      *SessionManager
	token   string // bearer token required on every request (Authorization: Bearer <token>)
	mu      sync.Mutex
	clients map[string][]*sseClient        // sessionID → subscribers
	streams map[string]*sessionStreamState // sessionID → stream readiness
}

// NewHTTPGateway creates an HTTPGateway backed by sm. A random bearer token is
// generated at construction; every request must present it via the
// Authorization header (see ServeHTTP). The token is exposed through Token() so
// the local operator can be shown the value on startup.
func NewHTTPGateway(sm *SessionManager) *HTTPGateway {
	return &HTTPGateway{
		sm:      sm,
		token:   newGatewayToken(),
		clients: make(map[string][]*sseClient),
		streams: make(map[string]*sessionStreamState),
	}
}

// Token returns the bearer token required to authenticate against the gateway.
func (g *HTTPGateway) Token() string {
	return g.token
}

// newGatewayToken returns a cryptographically random 32-byte token, hex-encoded.
func newGatewayToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand.Read never returns an error on supported platforms; if it
		// somehow does, panic rather than fall back to a predictable token.
		panic(fmt.Sprintf("acp: generating gateway token: %v", err))
	}
	return hex.EncodeToString(b)
}

// authorized reports whether r carries a valid "Authorization: Bearer <token>"
// header matching the gateway token, using a constant-time comparison.
func (g *HTTPGateway) authorized(r *http.Request) bool {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, prefix) {
		return false
	}
	got := strings.TrimPrefix(h, prefix)
	return subtle.ConstantTimeCompare([]byte(got), []byte(g.token)) == 1
}

func (g *HTTPGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Authenticate every request before any routing. The gateway drives an
	// agent that executes tools and reads/writes files on the operator's
	// machine, so an unauthenticated request must never be processed.
	if !g.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	path := r.URL.Path

	// POST /session
	if path == "/session" && r.Method == http.MethodPost {
		g.handleNewSession(w, r)
		return
	}

	// /session/{id}/...
	if strings.HasPrefix(path, "/session/") {
		rest := strings.TrimPrefix(path, "/session/")
		parts := strings.SplitN(rest, "/", 2)
		sessionID := parts[0]
		if len(parts) == 1 {
			// DELETE /session/{id}
			if r.Method == http.MethodDelete {
				g.handleCancelSession(w, r, sessionID)
				return
			}
		} else {
			sub := parts[1]
			switch {
			case sub == "prompt" && r.Method == http.MethodPost:
				g.handlePrompt(w, r, sessionID)
				return
			case sub == "stream" && r.Method == http.MethodGet:
				g.handleStream(w, r, sessionID)
				return
			}
		}
	}

	http.NotFound(w, r)
}

func (g *HTTPGateway) handleNewSession(w http.ResponseWriter, r *http.Request) {
	var p SessionNewParams
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	id, err := g.sm.NewSession(r.Context(), p.WorkingDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(SessionNewResult{SessionID: id}); err != nil {
		// Best-effort: headers already sent; log silently.
		_ = err
	}
}

// streamStateFor returns (creating if necessary) the sessionStreamState for id.
// Must be called without g.mu held.
func (g *HTTPGateway) streamStateFor(sessionID string) *sessionStreamState {
	g.mu.Lock()
	defer g.mu.Unlock()
	st, ok := g.streams[sessionID]
	if !ok {
		st = &sessionStreamState{readyCh: make(chan struct{})}
		g.streams[sessionID] = st
	}
	return st
}

func (g *HTTPGateway) handlePrompt(w http.ResponseWriter, r *http.Request, sessionID string) {
	var body struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Acquire the stream-state reference BEFORE the session lookup and hold it
	// across the cancel window. A concurrent handleCancelSession deletes the
	// stream-state map entry, but because we hold this *sessionStreamState
	// pointer its readyCh stays valid and the select below still unblocks the
	// moment the (already-registered) subscriber closed it — no TOCTOU stall.
	st := g.streamStateFor(sessionID)

	// Look the session up and capture its lifetime context in one shot. Using
	// the session pointer's context (rather than a second SessionCtx call after
	// the wait) closes the window where Cancel could delete the session between
	// the lookup and the context fetch.
	sess, err := g.sm.Lookup(sessionID)
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	promptCtx := sess.ctx

	// Wait until the SSE subscriber is registered so that broadcast never
	// races with an empty client list.  Use a short timeout so that a caller
	// who deliberately skips /stream does not block forever.
	waitCtx, waitCancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer waitCancel()
	select {
	case <-st.readyCh:
		// subscriber registered; safe to proceed
	case <-waitCtx.Done():
		// Timeout: no subscriber arrived. Proceed anyway (events will be
		// dropped), consistent with documented ordering requirements.
	}

	onEvent := func(ev AgentEvent) {
		var payload []byte
		var err error
		switch ev.Kind {
		case EventKindTextDelta:
			payload, err = json.Marshal(TextDeltaParams{SessionID: sessionID, Delta: ev.Text})
		case EventKindInfo:
			payload, err = json.Marshal(InfoParams{SessionID: sessionID, Text: ev.Text})
		case EventKindDone:
			p := DoneParams{SessionID: sessionID, StopReason: ev.StopReason}
			if ev.Err != nil {
				s := ev.Err.Error()
				p.Error = &s
			}
			payload, err = json.Marshal(p)
		}
		if err != nil {
			// Marshalling a known struct should never fail; skip on error.
			return
		}
		g.broadcast(sessionID, string(payload))
	}

	go func() {
		if err := g.sm.Prompt(promptCtx, sessionID, body.Prompt, onEvent); err != nil {
			// Broadcast a synthetic done event so SSE clients are not left hanging.
			errStr := err.Error()
			p := DoneParams{SessionID: sessionID, StopReason: "error", Error: &errStr}
			if b, merr := json.Marshal(p); merr == nil {
				g.broadcast(sessionID, string(b))
			}
		}
	}()

	// 202 Accepted: the work is dispatched asynchronously.
	w.WriteHeader(http.StatusAccepted)
}
func (g *HTTPGateway) handleStream(w http.ResponseWriter, r *http.Request, sessionID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	client := &sseClient{
		ch:     make(chan string, 64),
		cancel: cancel,
		ctx:    ctx,
	}

	// Register the client and signal readiness BEFORE writing headers so:
	// (a) any broadcast arriving between WriteHeader and the read loop is not lost, and
	// (b) handlePrompt's select-on-readyCh unblocks as soon as the client is
	//     registered, before the network round-trip to flush the headers completes.
	g.mu.Lock()
	g.clients[sessionID] = append(g.clients[sessionID], client)
	g.mu.Unlock()

	st := g.streamStateFor(sessionID)
	st.once.Do(func() { close(st.readyCh) })

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	defer func() {
		cancel()
		g.mu.Lock()
		list := g.clients[sessionID]
		for i, c := range list {
			if c == client {
				g.clients[sessionID] = append(list[:i], list[i+1:]...)
				break
			}
		}
		g.mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, open := <-client.ch:
			if !open {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}
	}
}

func (g *HTTPGateway) handleCancelSession(w http.ResponseWriter, r *http.Request, sessionID string) {
	g.sm.Cancel(sessionID)
	// Signal all SSE clients for this session to shut down by calling their
	// cancel function only.  We do NOT close c.ch here because broadcast may
	// hold a stale pointer to the client and a send on a closed channel panics;
	// the stream loop exits via ctx.Done() instead.
	g.mu.Lock()
	for _, c := range g.clients[sessionID] {
		c.cancel()
	}
	delete(g.clients, sessionID)
	delete(g.streams, sessionID)
	g.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

func (g *HTTPGateway) broadcast(sessionID, msg string) {
	g.mu.Lock()
	clients := make([]*sseClient, len(g.clients[sessionID]))
	copy(clients, g.clients[sessionID])
	g.mu.Unlock()
	for _, c := range clients {
		select {
		case <-c.ctx.Done():
			// client is shutting down; skip
		case c.ch <- msg:
			// delivered
		default:
			// Client's 64-slot buffer is full and it is not shutting down.
			// For SSE, dropping a frame to a stalled client is acceptable and
			// far preferable to blocking broadcast for ALL other clients. The
			// non-blocking default makes broadcast deadlock-free.
		}
	}
}
