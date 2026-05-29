package mcp

import "time"

// LifecycleState is the connection state of a single MCP server proxy.
type LifecycleState int

const (
	StateInitializing LifecycleState = iota
	StateConnected
	StateDegraded
	StateFailed
)

// String returns a human-readable state label.
func (s LifecycleState) String() string {
	switch s {
	case StateInitializing:
		return "initializing"
	case StateConnected:
		return "connected"
	case StateDegraded:
		return "degraded"
	case StateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// ServerProxy wraps a single MCP server: transport + capabilities +
// discovered tools. Callers interact through the Registry, not directly.
//
// The liveness/backoff bookkeeping fields (reconnectAttempted,
// backoffUntil, command, args, env) are carried per-proxy but are
// guarded exclusively by Registry.mu — there is intentionally NO
// per-proxy lock. The proxy is replaced wholesale on reconnect and is
// only ever read or written while holding Registry.mu, so a per-proxy
// lock would create lock-ordering hazards with Registry.mu and is
// unnecessary.
type ServerProxy struct {
	Name  string
	State LifecycleState
	Caps  ServerCapabilities
	Tools []McpToolMeta
	t     Transport

	// Reconnect inputs, captured at Connect so the watcher can rebuild
	// the transport on process death without re-reading config.
	command string
	args    []string
	env     map[string]string

	// reconnectAttempted is true once the single bounded reconnect has
	// run for the current degraded incident. Guarded by Registry.mu.
	reconnectAttempted bool
	// backoffUntil is the negative-result cooldown gate: a failed
	// reconnect sets it into the future and no further automatic dial is
	// attempted until it elapses. Zero means no gate. Guarded by
	// Registry.mu.
	backoffUntil time.Time
}

// Close tears down the proxy's transport.
func (s *ServerProxy) Close() error {
	if s.t != nil {
		return s.t.Close()
	}
	return nil
}
