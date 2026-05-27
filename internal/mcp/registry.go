package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Registry manages the lifecycle of multiple MCP servers and provides
// unified tool discovery and calling.
type Registry struct {
	mu       sync.RWMutex
	servers  map[string]*ServerProxy
	timeouts map[string]int // per-server call timeout in seconds
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		servers:  make(map[string]*ServerProxy),
		timeouts: make(map[string]int),
	}
}

// Connect spawns an MCP server, runs initialize, discovers tools, and
// registers it under the given name. Name must be unique.
func (r *Registry) Connect(ctx context.Context, name, command string, args []string, env map[string]string) error {
	r.mu.Lock()
	if _, ok := r.servers[name]; ok {
		r.mu.Unlock()
		return fmt.Errorf("mcp server %q already registered", name)
	}
	// Reserve the slot while we initialise so concurrent connects don't
	// race for the same name.
	r.servers[name] = &ServerProxy{Name: name, State: StateInitializing}
	r.mu.Unlock()

	transport, err := NewStdioTransport(ctx, command, args, env)
	if err != nil {
		r.mu.Lock()
		r.servers[name].State = StateFailed
		r.mu.Unlock()
		return fmt.Errorf("mcp %q: spawn: %w", name, err)
	}

	caps, err := initialize(ctx, transport)
	if err != nil {
		transport.Close()
		r.mu.Lock()
		r.servers[name].State = StateFailed
		r.mu.Unlock()
		return fmt.Errorf("mcp %q: initialize: %w", name, err)
	}

	tools, err := listTools(ctx, transport)
	if err != nil {
		transport.Close()
		r.mu.Lock()
		r.servers[name].State = StateFailed
		r.mu.Unlock()
		return fmt.Errorf("mcp %q: tools/list: %w", name, err)
	}

	r.mu.Lock()
	r.servers[name] = &ServerProxy{
		Name:  name,
		State: StateConnected,
		Caps:  caps,
		Tools: tools,
		t:     transport,
	}
	r.mu.Unlock()
	return nil
}

// Shutdown closes all server transports. Each gets up to 5s to exit
// before the context cancels; a best-effort Kill is already in Close.
func (r *Registry) Shutdown() {
	r.mu.RLock()
	proxies := make([]*ServerProxy, 0, len(r.servers))
	for _, s := range r.servers {
		proxies = append(proxies, s)
	}
	r.mu.RUnlock()

	var wg sync.WaitGroup
	for _, s := range proxies {
		wg.Add(1)
		go func(sp *ServerProxy) {
			defer wg.Done()
			_ = sp.Close()
		}(s)
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
}

// Servers returns a snapshot of all registered server proxies. The
// returned slice is safe to read but must not be mutated.
func (r *Registry) Servers() []*ServerProxy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*ServerProxy, 0, len(r.servers))
	for _, s := range r.servers {
		out = append(out, s)
	}
	return out
}

// prefixSep separates the server name from the tool name in the
// fully-qualified MCP tool name: mcp__<server>__<tool>.
const mcpPrefix = "mcp__"

// Tools returns all MCP tools with their names prefixed as
// "mcp__<server>__<tool>".
func (r *Registry) Tools() []McpToolMeta {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []McpToolMeta
	for _, s := range r.servers {
		if s.State != StateConnected {
			continue
		}
		for _, t := range s.Tools {
			fullName := mcpPrefix + s.Name + "__" + t.Name
			out = append(out, McpToolMeta{
				Name:        fullName,
				Description: t.Description,
				InputSchema: t.InputSchema,
			})
		}
	}
	return out
}

// CallTool dispatches a fully-qualified tool name to the right server
// and returns the content, isError flag, and any infrastructure error.
func (r *Registry) CallTool(ctx context.Context, fullName string, args json.RawMessage) (string, bool, error) {
	if !strings.HasPrefix(fullName, mcpPrefix) {
		return "", false, fmt.Errorf("unknown mcp tool: %s", fullName)
	}
	rest := strings.TrimPrefix(fullName, mcpPrefix)
	sep := strings.Index(rest, "__")
	if sep < 0 {
		return "", false, fmt.Errorf("unknown mcp tool: %s", fullName)
	}
	serverName := rest[:sep]
	toolName := rest[sep+2:]

	r.mu.RLock()
	srv, ok := r.servers[serverName]
	r.mu.RUnlock()
	if !ok || srv.State != StateConnected {
		return "", false, fmt.Errorf("mcp server %q not connected", serverName)
	}

	// Apply per-call timeout (server-specific or default 60s).
	timeoutSec := 60
	r.mu.RLock()
	if ts, ok := r.timeouts[serverName]; ok && ts > 0 {
		timeoutSec = ts
	}
	r.mu.RUnlock()
	tctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	return callTool(tctx, srv.t, toolName, args)
}

// SetTimeout configures the per-call timeout for a given server.
func (r *Registry) SetTimeout(name string, seconds int) {
	r.mu.Lock()
	r.timeouts[name] = seconds
	r.mu.Unlock()
}

// MCPChange describes a tool-level mutation detected between two schema snapshots.
type MCPChange struct {
	Kind     string // "tool_added", "tool_removed", "tool_schema_changed"
	ToolName string
}

// SchemaHash returns a deterministic SHA-256 hex digest of all current
// MCP tool schemas (sorted by fully-qualified name). Same tools with
// same schemas produce the same hash regardless of connection order.
func (r *Registry) SchemaHash() string {
	tools := r.Tools()
	if len(tools) == 0 {
		return sha256hex("")
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name < tools[j].Name
	})
	var sb strings.Builder
	for _, t := range tools {
		sb.WriteString(t.Name)
		sb.WriteByte(':')
		sb.WriteString(t.Description)
		sb.WriteByte(':')
		sb.Write(t.InputSchema)
		sb.WriteByte('\n')
	}
	return sha256hex(sb.String())
}

// PendingSchemaChanges compares the current tool set against a previous
// tool list snapshot and returns the list of MCP-level changes.
func (r *Registry) PendingSchemaChanges(oldTools []McpToolMeta) []MCPChange {
	currentTools := r.Tools()

	oldByName := make(map[string]McpToolMeta, len(oldTools))
	for _, t := range oldTools {
		oldByName[t.Name] = t
	}
	currentByName := make(map[string]McpToolMeta, len(currentTools))
	for _, t := range currentTools {
		currentByName[t.Name] = t
	}

	var changes []MCPChange
	for name := range currentByName {
		if _, existed := oldByName[name]; !existed {
			changes = append(changes, MCPChange{Kind: "tool_added", ToolName: name})
		}
	}
	for name := range oldByName {
		if _, exists := currentByName[name]; !exists {
			changes = append(changes, MCPChange{Kind: "tool_removed", ToolName: name})
		}
	}
	for name, cur := range currentByName {
		if old, existed := oldByName[name]; existed {
			if old.Description != cur.Description || !schemasEqualBytes(old.InputSchema, cur.InputSchema) {
				changes = append(changes, MCPChange{Kind: "tool_schema_changed", ToolName: name})
			}
		}
	}
	return changes
}

func schemasEqualBytes(a, b json.RawMessage) bool {
	return schemasEqual(a, b)
}

func sha256hex(s string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(s)))
}
