// internal/mcp/streamable_http.go
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// StreamableHTTPTransport implements Transport for MCP "Streamable HTTP"
// (spec 2025-03-26): one endpoint, JSON-RPC over POST, with an Mcp-Session-Id
// header the server establishes and the client echoes on later requests.
type StreamableHTTPTransport struct {
	url       string
	client    *http.Client
	nextID    atomic.Int64
	done      chan struct{}
	mu        sync.Mutex
	sessionID string
}

func NewStreamableHTTPTransport(url string) *StreamableHTTPTransport {
	return &StreamableHTTPTransport{
		url:    url,
		client: &http.Client{Timeout: 60 * time.Second},
		done:   make(chan struct{}),
	}
}

func (t *StreamableHTTPTransport) Send(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := t.nextID.Add(1)
	body, err := json.Marshal(jsonRPCRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params})
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

func (t *StreamableHTTPTransport) Notify(ctx context.Context, method string, params any) error {
	body, err := json.Marshal(jsonRPCNotification{JSONRPC: "2.0", Method: method, Params: params})
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}
	_, err = t.post(ctx, body)
	return err
}

func (t *StreamableHTTPTransport) post(ctx context.Context, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	t.mu.Lock()
	if t.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", t.sessionID)
	}
	t.mu.Unlock()

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		t.mu.Lock()
		t.sessionID = sid
		t.mu.Unlock()
	}
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("mcp http status %d: %s", resp.StatusCode, string(out))
	}
	return out, nil
}

func (t *StreamableHTTPTransport) Done() <-chan struct{} { return t.done }

func (t *StreamableHTTPTransport) Close() error {
	select {
	case <-t.done:
	default:
		close(t.done)
	}
	return nil
}
