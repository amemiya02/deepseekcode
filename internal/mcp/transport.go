package mcp

import (
	"context"
	"encoding/json"
)

// Transport abstracts the JSON-RPC message channel to an MCP server.
// Implementations handle the wire format (stdio, HTTP, SSE) and
// lifecycle (connect, close). Callers see only Send/Notify/Close.
type Transport interface {
	// Send sends a request and blocks until the matching response
	// arrives or ctx is cancelled.
	Send(ctx context.Context, method string, params any) (json.RawMessage, error)

	// Notify fires a notification (no response expected).
	Notify(ctx context.Context, method string, params any) error

	// Done returns a channel that is closed when the transport's
	// underlying connection (subprocess, SSE stream) has ended. The
	// Registry's liveness watcher selects on this to detect server
	// death. Transports that do not have a liveness signal (e.g.
	// stateless HTTP) should return a channel that is never closed.
	Done() <-chan struct{}

	// Close tears down the transport and releases resources.
	Close() error
}
