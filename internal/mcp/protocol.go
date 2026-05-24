package mcp

import "encoding/json"

// ProtocolVersion is the MCP protocol version this client advertises
// during the initialize handshake.
const ProtocolVersion = "2025-06-18"

// jsonRPCRequest is the JSON-RPC 2.0 request envelope sent to the server.
type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// jsonRPCResponse is the JSON-RPC 2.0 response envelope received from
// the server. Result is non-nil on success; Error is non-nil on failure.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

// jsonRPCError is a JSON-RPC 2.0 error object.
type jsonRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// jsonRPCNotification is a JSON-RPC 2.0 notification (id=0, no response).
type jsonRPCNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}
