package mcp

import (
	"context"
	"encoding/json"
)

// Transport abstracts the JSON-RPC message channel to an MCP server.
// Implementations handle the wire format (stdio, HTTP, etc.) and
// lifecycle (connect, close). Callers see only Send/Notify/Close.
type Transport interface {
	// Send sends a request and blocks until the matching response
	// arrives or ctx is cancelled.
	Send(ctx context.Context, method string, params any) (json.RawMessage, error)

	// Notify fires a notification (no response expected).
	Notify(ctx context.Context, method string, params any) error

	// Close tears down the transport and releases resources.
	Close() error
}
