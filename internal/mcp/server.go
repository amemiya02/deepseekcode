package mcp

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
type ServerProxy struct {
	Name  string
	State LifecycleState
	Caps  ServerCapabilities
	Tools []McpToolMeta
	t     Transport
}

// Close tears down the proxy's transport.
func (s *ServerProxy) Close() error {
	if s.t != nil {
		return s.t.Close()
	}
	return nil
}
