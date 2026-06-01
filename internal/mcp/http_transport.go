package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// HTTPTransport implements Transport for MCP servers reachable via
// HTTP POST (JSON-RPC request/response). It does not use SSE and is
// suitable for servers that accept stateless JSON-RPC over HTTP.
type HTTPTransport struct {
	url    string
	client *http.Client
	nextID atomic.Int64
	done   chan struct{} // never closed; stateless HTTP has no liveness signal
}

func NewHTTPTransport(url string) *HTTPTransport {
	return &HTTPTransport{
		url:    url,
		client: &http.Client{Timeout: 30 * time.Second},
		done:   make(chan struct{}),
	}
}

func (t *HTTPTransport) Send(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := t.nextID.Add(1)
	body, err := json.Marshal(jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	respBody, err := t.post(ctx, body)
	if err != nil {
		return nil, err
	}
	var resp jsonRPCResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("mcp error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	return resp.Result, nil
}

func (t *HTTPTransport) Notify(ctx context.Context, method string, params any) error {
	body, err := json.Marshal(jsonRPCNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}
	_, err = t.post(ctx, body)
	return err
}

func (t *HTTPTransport) post(ctx context.Context, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("mcp http status %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func (t *HTTPTransport) Close() error {
	return nil
}

// Done returns a channel that is never closed. Stateless HTTP has no
// liveness signal — the server is always considered reachable.
func (t *HTTPTransport) Done() <-chan struct{} {
	return t.done
}

// --- SSE Transport ---

// SSETransport implements Transport for MCP servers using Server-Sent
// Events (SSE) for server→client messages and HTTP POST for
// client→server messages.
//
// Lifecycle:
//  1. GET the SSE URL to open the event stream.
//  2. The server sends an "endpoint" event containing the POST URL.
//  3. All subsequent Send calls POST JSON-RPC to that URL.
//  4. Responses arrive as "message" events on the SSE stream,
//     routed by request ID to waiting Send callers.
//
// If the SSE stream disconnects, the transport is dead and the
// Registry's watcher detects it via Done(). The Registry handles
// reconnect (not the transport itself).
type SSETransport struct {
	sseURL    string          // the URL we GET for the SSE stream
	postURL   string          // discovered from the "endpoint" event
	client    *http.Client    // shared HTTP client for POST
	nextID    atomic.Int64
	done      chan struct{}   // closed when SSE stream or reader exits
	closeOnce sync.Once
	closeErr  error

	mu       sync.Mutex
	pending  map[int64]chan jsonRPCResponse
	pendingMu sync.Mutex

	endpointReady chan struct{} // closed once postURL is known
}

// NewSSETransport creates an SSE transport. The SSE stream is NOT
// started here — call Start to begin reading. This two-phase init
// lets the caller wire the transport into the Registry before any
// goroutines start.
func NewSSETransport(sseURL string) *SSETransport {
	return &SSETransport{
		sseURL:        sseURL,
		client:        &http.Client{Timeout: 60 * time.Second},
		done:          make(chan struct{}),
		pending:       make(map[int64]chan jsonRPCResponse),
		endpointReady: make(chan struct{}),
	}
}

// Start opens the SSE connection and begins reading events. It blocks
// until the "endpoint" event is received (so postURL is known) or ctx
// is cancelled.
func (t *SSETransport) Start(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.sseURL, nil)
	if err != nil {
		return fmt.Errorf("sse request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("sse connect: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return fmt.Errorf("sse status %d", resp.StatusCode)
	}

	// Read the SSE stream in a background goroutine. The first event
	// must be "endpoint" — we wait for it here so Start returns only
	// after the transport is fully usable.
	endpointCh := make(chan string, 1)
	endpointErr := make(chan error, 1)
	go t.readSSE(resp.Body, endpointCh, endpointErr)

	select {
	case url := <-endpointCh:
		t.postURL = url
		close(t.endpointReady)
		return nil
	case err := <-endpointErr:
		return err
	case <-ctx.Done():
		resp.Body.Close()
		return ctx.Err()
	}
}

// Done returns a channel closed when the SSE stream has ended. The
// Registry's liveness watcher selects on this to detect server death.
func (t *SSETransport) Done() <-chan struct{} {
	return t.done
}

// Send posts a JSON-RPC request and waits for the response to arrive
// as an SSE "message" event.
func (t *SSETransport) Send(ctx context.Context, method string, params any) (json.RawMessage, error) {
	// Wait for the endpoint to be discovered.
	select {
	case <-t.endpointReady:
	case <-t.done:
		return nil, fmt.Errorf("sse transport closed")
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	id := t.nextID.Add(1)
	body, err := json.Marshal(jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	ch := make(chan jsonRPCResponse, 1)
	t.pendingMu.Lock()
	t.pending[id] = ch
	t.pendingMu.Unlock()

	defer func() {
		t.pendingMu.Lock()
		delete(t.pending, id)
		t.pendingMu.Unlock()
	}()

	if err := t.postMessage(ctx, body); err != nil {
		return nil, err
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("mcp error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	case <-t.done:
		return nil, fmt.Errorf("sse transport closed")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Notify posts a JSON-RPC notification (no response expected).
func (t *SSETransport) Notify(ctx context.Context, method string, params any) error {
	select {
	case <-t.endpointReady:
	case <-t.done:
		return fmt.Errorf("sse transport closed")
	case <-ctx.Done():
		return ctx.Err()
	}

	body, err := json.Marshal(jsonRPCNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}
	return t.postMessage(ctx, body)
}

// Close shuts down the SSE transport. The SSE reader goroutine will
// detect the closed body and exit.
func (t *SSETransport) Close() error {
	t.closeOnce.Do(func() {
		// Signal the reader goroutine and unblock any pending Send calls.
		close(t.done)
		// Drain pending channels so no Send blocks forever.
		t.pendingMu.Lock()
		for id, ch := range t.pending {
			ch <- jsonRPCResponse{
				ID: id,
				Error: &jsonRPCError{
					Code:    -32000,
					Message: "sse transport closed",
				},
			}
		}
		t.pendingMu.Unlock()
	})
	return t.closeErr
}

func (t *SSETransport) postMessage(ctx context.Context, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.postURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("sse post: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("sse post: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sse post status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// readLine reads one line from r, stripping the trailing \n or \r\n.
// Returns the line without the newline delimiter. An empty slice means
// a blank line (the SSE event boundary).
func readLine(r io.Reader) ([]byte, error) {
	var line []byte
	buf := make([]byte, 1)
	for {
		_, err := r.Read(buf)
		if err != nil {
			if len(line) > 0 {
				return line, nil
			}
			return nil, err
		}
		if buf[0] == '\n' {
			// Strip trailing \r if present.
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			return line, nil
		}
		line = append(line, buf[0])
	}
}

// --- Helpers shared by both HTTP and SSE transports ---

// sseURLFromServerURL converts a bare MCP server URL into the SSE
// endpoint by appending "/sse" if not already present. This is the
// common convention for MCP SSE servers.
func sseURLFromServerURL(u string) string {
	if strings.HasSuffix(u, "/sse") {
		return u
	}
	return strings.TrimRight(u, "/") + "/sse"
}

// (which carry the POST URL) and "message" events (which carry
// JSON-RPC responses). The first "endpoint" event is sent on
// endpointCh; subsequent events are routed by ID to pending Send
// callers. Errors before the first endpoint are sent on endpointErr.
func (t *SSETransport) readSSE(body io.ReadCloser, endpointCh chan<- string, endpointErr chan<- error) {
	defer body.Close()
	defer close(t.done) // ensure Done is always closed

	var eventType, dataBuf string

	// Simple SSE parser: accumulate event type and data lines,
	// dispatch on blank line.
	for {
		lineBytes, err := readLine(body)
		if err != nil {
			// Stream ended or error — transport is dead.
			select {
			case endpointErr <- fmt.Errorf("sse stream ended: %w", err):
			default:
			}
			return
		}
		line := string(lineBytes)

		switch {
		case len(line) == 0:
			// Blank line = event boundary. Dispatch accumulated event.
			if dataBuf == "" {
				eventType = ""
				continue
			}
			if eventType == "endpoint" {
				select {
				case endpointCh <- dataBuf:
				default:
				}
			} else if eventType == "message" || eventType == "" {
				// Default event type for SSE is "message" if not specified.
				var resp jsonRPCResponse
				if err := json.Unmarshal([]byte(dataBuf), &resp); err == nil && resp.ID != 0 {
					t.pendingMu.Lock()
					ch, ok := t.pending[resp.ID]
					t.pendingMu.Unlock()
					if ok {
						ch <- resp
					}
				}
			}
			eventType = ""
			dataBuf = ""
		case strings.HasPrefix(line, "event:"):
			eventType = strings.TrimPrefix(line, "event:")
			eventType = strings.TrimSpace(eventType)
		case strings.HasPrefix(line, "data:"):
			dataBuf = strings.TrimPrefix(line, "data:")
			// Preserve leading space if present (SSE spec: single space
			// after "data:" is stripped; we keep the rest).
			if len(dataBuf) > 0 && dataBuf[0] == ' ' {
				dataBuf = dataBuf[1:]
			}
		// Ignore "id:", "retry:", comments (lines starting with ":")
		default:
			// Unknown field — ignore per SSE spec.
		}
	}
}
