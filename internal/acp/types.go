package acp

import (
	"encoding/json"
	"errors"
	"fmt"
)

// JSONRPC20 is the protocol version literal required in every message.
const JSONRPC20 = "2.0"

// ID is a JSON-RPC 2.0 message identifier (integer, string, or null).
// It is a type alias for json.RawMessage to remain compatible with callers
// that assign json.RawMessage literals directly.
type ID = json.RawMessage

// NewID creates an integer ID from n.
func NewID(n int64) ID {
	b, _ := json.Marshal(n)
	return ID(b)
}

// Compile-time assertion: *RPCError must satisfy the error interface.
var _ error = (*RPCError)(nil)

// Request is a JSON-RPC 2.0 request (has id + method).
// ACP restricts IDs to integer values; the field is json.RawMessage to
// remain compatible with peers that send string or null IDs per §4 of
// the JSON-RPC 2.0 specification.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Validate returns an error if the message violates JSON-RPC 2.0 invariants.
func (r *Request) Validate() error {
	if r.JSONRPC != JSONRPC20 {
		return fmt.Errorf("acp: jsonrpc must be %q, got %q", JSONRPC20, r.JSONRPC)
	}
	return nil
}

// Notification is a JSON-RPC 2.0 notification (method, no id).
type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Validate returns an error if the message violates JSON-RPC 2.0 invariants.
func (n *Notification) Validate() error {
	if n.JSONRPC != JSONRPC20 {
		return fmt.Errorf("acp: jsonrpc must be %q, got %q", JSONRPC20, n.JSONRPC)
	}
	return nil
}

// Response is a JSON-RPC 2.0 response (id, no method).
// Per §5, Result and Error are mutually exclusive: exactly one must be
// non-nil in a well-formed response. Validate() enforces this constraint.
// ID is json.RawMessage to accept integer, string, or null values per §4.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// Validate returns an error if the message violates JSON-RPC 2.0 invariants.
func (r *Response) Validate() error {
	if r.JSONRPC != JSONRPC20 {
		return fmt.Errorf("acp: jsonrpc must be %q, got %q", JSONRPC20, r.JSONRPC)
	}
	if r.Result != nil && r.Error != nil {
		return errors.New("acp: response Result and Error are mutually exclusive")
	}
	return nil
}

// RPCError is a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error implements the error interface.
func (e *RPCError) Error() string {
	return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message)
}

// Error codes (JSON-RPC spec + ACP extensions).
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// ACP-domain params/result types.

// SessionNewParams is the params for session/new.
type SessionNewParams struct {
	WorkingDir string `json:"workingDir"`
}

// SessionNewResult is the result for session/new.
type SessionNewResult struct {
	SessionID string `json:"sessionId"`
}

// SessionPromptParams is the params for session/prompt.
type SessionPromptParams struct {
	SessionID string `json:"sessionId"`
	Prompt    string `json:"prompt"`
}

// SessionCancelParams is the params for session/cancel.
type SessionCancelParams struct {
	SessionID string `json:"sessionId"`
}

// Notification param types (server → client).

// TextDeltaParams is the params for session/textDelta notifications.
type TextDeltaParams struct {
	SessionID string `json:"sessionId"`
	Delta     string `json:"delta"`
}

// InfoParams is the params for session/info notifications.
type InfoParams struct {
	SessionID string `json:"sessionId"`
	Text      string `json:"text"`
}

// DoneParams is the params for session/done notifications.
// Error is a pointer so that a missing field (nil) is unambiguously
// distinct from an intentional empty-string error value.
type DoneParams struct {
	SessionID  string  `json:"sessionId"`
	StopReason string  `json:"stopReason"`
	Error      *string `json:"error,omitempty"`
}

// ToolCallParams is the params for session/toolCall notifications (server→client).
type ToolCallParams struct {
	SessionID string          `json:"sessionId"`
	CallID    string          `json:"callId"`
	Tool      string          `json:"tool"`
	Input     json.RawMessage `json:"input"`
}

// PermissionParams is the params for session/permissionRequest notifications.
type PermissionParams struct {
	SessionID   string `json:"sessionId"`
	RequestID   string `json:"requestId"`
	Description string `json:"description"`
}
