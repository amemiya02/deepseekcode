package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// dialResult bundles the outputs of a successful dial: the live
// transport plus the capabilities and tools discovered during the
// handshake.
type dialResult struct {
	t     Transport
	caps  ServerCapabilities
	tools []McpToolMeta
}

// transportKind identifies which transport type a server uses, so the
// reconnect path can recreate it.
type transportKind int

const (
	transportStdio transportKind = iota
	transportSSE
)

// Registry manages the lifecycle of multiple MCP servers and provides
// unified tool discovery and calling.
//
// LOCK MODEL: r.mu (the RWMutex) is the SOLE authority over r.servers
// and every state transition (markDegraded, the reconnect commit, the
// backoff fields). There is intentionally no per-ServerProxy lock.
// Network/IO (NewStdioTransport/initialize/listTools, performed by dial)
// MUST run WITHOUT r.mu held; only the final commit re-takes r.mu and
// re-validates state before installing a new transport.
type Registry struct {
	mu       sync.RWMutex
	servers  map[string]*ServerProxy
	timeouts map[string]int // per-server call timeout in seconds

	// Watcher lifecycle. Each connected StdioTransport owns exactly one
	// watcher goroutine bound to that transport's Done() channel; wg
	// tracks them and Shutdown closes done (once) to stop them.
	wg            sync.WaitGroup
	done          chan struct{}
	closeDoneOnce sync.Once

	// ctx is cancelled by Shutdown so in-flight dial attempts in
	// attemptReconnect abort promptly instead of blocking until their
	// own timeout expires.
	ctx    context.Context
	cancel context.CancelFunc

	// reconnectBackoff is the negative-result cooldown applied after a
	// failed reconnect; a flapping server must not re-dial within it.
	reconnectBackoff time.Duration
	// reconnectTimeout bounds the single automatic reconnect attempt.
	reconnectTimeout time.Duration

	// dialer performs the spawn+initialize+listTools sequence. It is a
	// seam: defaulted to the real dial, overridable in-package by tests
	// to inject a pipe transport instead of exec'ing a real process.
	dialer func(ctx context.Context, name, command string, args []string, env map[string]string) (dialResult, error)
	// sseDialer performs the SSE connect+initialize+listTools sequence.
	// Seam for testing, analogous to dialer.
	sseDialer func(ctx context.Context, name, sseURL string) (dialResult, error)
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	ctx, cancel := context.WithCancel(context.Background())
	r := &Registry{
		servers:          make(map[string]*ServerProxy),
		timeouts:         make(map[string]int),
		done:             make(chan struct{}),
		ctx:              ctx,
		cancel:           cancel,
		reconnectBackoff: 30 * time.Second,
		reconnectTimeout: 10 * time.Second,
	}
	r.dialer = r.dial
	r.sseDialer = r.dialSSE
	return r
}

// dial performs the spawn + initialize + listTools sequence and returns
// a live transport with its discovered capabilities and tools. It does
// NOT touch r.servers or r.mu — it is pure IO, shared by Connect and
// attemptReconnect so their wire behavior cannot diverge. The caller is
// responsible for closing the returned transport on a later failure.
func (r *Registry) dial(ctx context.Context, name, command string, args []string, env map[string]string) (dialResult, error) {
	if command == "" {
		return dialResult{}, fmt.Errorf("mcp %q: empty command", name)
	}

	transport, err := NewStdioTransport(ctx, command, args, env)
	if err != nil {
		return dialResult{}, fmt.Errorf("mcp %q: spawn: %w", name, err)
	}

	caps, err := initialize(ctx, transport)
	if err != nil {
		transport.Close()
		return dialResult{}, fmt.Errorf("mcp %q: initialize: %w", name, err)
	}

	tools, err := listTools(ctx, transport)
	if err != nil {
		transport.Close()
		return dialResult{}, fmt.Errorf("mcp %q: tools/list: %w", name, err)
	}

	return dialResult{t: transport, caps: caps, tools: tools}, nil
}

// dialSSE connects to an MCP server via SSE transport. It opens the
// SSE stream, runs initialize and listTools over it, and returns the
// live transport.
func (r *Registry) dialSSE(ctx context.Context, name, sseURL string) (dialResult, error) {
	if sseURL == "" {
		return dialResult{}, fmt.Errorf("mcp %q: empty url for sse transport", name)
	}

	transport := NewSSETransport(sseURL)
	if err := transport.Start(ctx); err != nil {
		return dialResult{}, fmt.Errorf("mcp %q: sse connect: %w", name, err)
	}

	caps, err := initialize(ctx, transport)
	if err != nil {
		transport.Close()
		return dialResult{}, fmt.Errorf("mcp %q: initialize: %w", name, err)
	}

	tools, err := listTools(ctx, transport)
	if err != nil {
		transport.Close()
		return dialResult{}, fmt.Errorf("mcp %q: tools/list: %w", name, err)
	}

	return dialResult{t: transport, caps: caps, tools: tools}, nil
}

// Connect spawns an MCP server, runs initialize, discovers tools, and
// registers it under the given name. Name must be unique. This is the
// stdio transport path — use ConnectSSE for SSE servers.
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

	res, err := r.dialer(ctx, name, command, args, env)
	if err != nil {
		r.mu.Lock()
		r.servers[name].State = StateFailed
		r.servers[name].lastError = err.Error()
		r.mu.Unlock()
		return err
	}

	r.mu.Lock()
	r.servers[name] = &ServerProxy{
		Name:    name,
		State:   StateConnected,
		Caps:    res.caps,
		Tools:   res.tools,
		t:       res.t,
		command: command,
		args:    args,
		env:     env,
		kind:    transportStdio,
	}
	r.mu.Unlock()

	// Spawn the liveness watcher bound to THIS transport's Done channel.
	r.wg.Add(1)
	go r.watch(name, res.t.Done())
	return nil
}

// ConnectSSE connects to an MCP server via SSE transport. It opens the
// SSE event stream, runs initialize, discovers tools, and registers the
// server under the given name. Name must be unique.
func (r *Registry) ConnectSSE(ctx context.Context, name, sseURL string) error {
	r.mu.Lock()
	if _, ok := r.servers[name]; ok {
		r.mu.Unlock()
		return fmt.Errorf("mcp server %q already registered", name)
	}
	r.servers[name] = &ServerProxy{Name: name, State: StateInitializing}
	r.mu.Unlock()

	res, err := r.sseDialer(ctx, name, sseURL)
	if err != nil {
		r.mu.Lock()
		r.servers[name].State = StateFailed
		r.servers[name].lastError = err.Error()
		r.mu.Unlock()
		return err
	}

	r.mu.Lock()
	r.servers[name] = &ServerProxy{
		Name:   name,
		State:  StateConnected,
		Caps:   res.caps,
		Tools:  res.tools,
		t:      res.t,
		kind:   transportSSE,
		sseURL: sseURL,
	}
	r.mu.Unlock()

	r.wg.Add(1)
	go r.watch(name, res.t.Done())
	return nil
}

// watch is the per-transport liveness watcher. It is bound to a specific
// transport's Done channel (passed by value), so an old watcher can never
// observe or degrade a newer transport. It runs exactly one reconnect
// attempt per process-death incident and then returns — it does NOT loop.
// The "single bounded reconnect" guarantee is structural: each connected
// transport gets one watcher, and a successful reconnect installs a NEW
// transport+watcher (so recovery is per-incident); a failed reconnect
// sets backoffUntil and the watcher exits, leaving the server degraded.
func (r *Registry) watch(name string, transportDone <-chan struct{}) {
	defer r.wg.Done()
	select {
	case <-r.done:
		// Registry shutting down — stop without touching state.
		return
	case <-transportDone:
		// The transport's reader goroutine exited: process is dead.
		r.markDegraded(name)
		r.attemptReconnect(name)
	}
}

// markDegraded flips a still-connected proxy to StateDegraded and clears
// its Tools so Registry.Tools() (which skips State != StateConnected)
// immediately stops advertising them. This is the Capability-Set drift
// trigger: the removal surfaces on the next agent turn via
// buildCapabilitySet / CompareToolLists as a PendingChange, NOT as live
// fingerprint movement (the frozen epoch keeps serving epoch.FrozenTools).
func (r *Registry) markDegraded(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	proxy, ok := r.servers[name]
	if !ok || proxy.State != StateConnected {
		return
	}
	proxy.State = StateDegraded
	proxy.Tools = nil // belt-and-suspenders; Tools() already filters by State.
	proxy.reconnectAttempted = false
}

// attemptReconnect runs the single bounded reconnect for a degraded
// server. The dial IO runs WITHOUT r.mu held; r.mu is only re-taken for
// the gate read and the final commit. It enforces:
//   - exactly one automatic attempt per degraded incident
//     (reconnectAttempted gate), and
//   - a negative-result cooldown (backoffUntil) so a flapping server is
//     not re-dialed within the window.
func (r *Registry) attemptReconnect(name string) {
	// Gate read under r.mu — no IO here.
	r.mu.RLock()
	proxy, ok := r.servers[name]
	if !ok || proxy.State != StateDegraded || proxy.reconnectAttempted || time.Now().Before(proxy.backoffUntil) {
		r.mu.RUnlock()
		return
	}
	kind := proxy.kind
	command := proxy.command
	args := proxy.args
	env := proxy.env
	sseURL := proxy.sseURL
	r.mu.RUnlock()

	// Dial without holding r.mu so the spawn/handshake can't block the
	// lock and stall the rest of the registry.
	dialCtx, cancel := context.WithTimeout(r.ctx, r.reconnectTimeout)
	defer cancel()

	var res dialResult
	var err error
	switch kind {
	case transportSSE:
		res, err = r.sseDialer(dialCtx, name, sseURL)
	default:
		res, err = r.dialer(dialCtx, name, command, args, env)
	}

	if err != nil {
		// Failure: record the single attempt and arm the cooldown gate.
		r.mu.Lock()
		if p, ok := r.servers[name]; ok && p.State == StateDegraded {
			p.reconnectAttempted = true
			p.backoffUntil = time.Now().Add(r.reconnectBackoff)
			p.lastError = err.Error()
		}
		r.mu.Unlock()
		return
	}

	// Success: re-validate under r.mu before committing the new
	// transport. If a concurrent Shutdown/Connect changed the state, or
	// the registry is shutting down, discard the fresh transport.
	select {
	case <-r.done:
		res.t.Close()
		return
	default:
	}

	r.mu.Lock()
	proxy, ok = r.servers[name]
	if !ok || proxy.State != StateDegraded || proxy.reconnectAttempted {
		r.mu.Unlock()
		res.t.Close()
		return
	}
	// Preserve tool filters from the old proxy across reconnect.
	oldEnabled := proxy.enabledTools
	oldDisabled := proxy.disabledTools
	r.servers[name] = &ServerProxy{
		Name:          name,
		State:         StateConnected,
		Caps:          res.caps,
		Tools:         res.tools,
		t:             res.t,
		command:       command,
		args:          args,
		env:           env,
		kind:          kind,
		sseURL:        sseURL,
		enabledTools:  oldEnabled,
		disabledTools: oldDisabled,
	}
	r.mu.Unlock()

	// New transport gets a new watcher bound to its own Done channel.
	r.wg.Add(1)
	go r.watch(name, res.t.Done())
}

// Shutdown stops all watchers and closes all server transports. Order:
//  1. close r.done (once) to signal every watcher to exit;
//  2. close each transport (which also closes its readerDone, unblocking
//     any watcher still parked in its select);
//  3. r.wg.Wait() within a deadline so no watcher goroutine outlives
//     Shutdown — this is what keeps -race and goroutine-leak detection
//     clean.
//
// Double Shutdown is safe: closeDoneOnce guards close(r.done).
func (r *Registry) Shutdown() {
	// Signal watchers to exit. Guarded so a double Shutdown is safe.
	r.closeDoneOnce.Do(func() { close(r.done); r.cancel() })

	r.mu.RLock()
	proxies := make([]*ServerProxy, 0, len(r.servers))
	for _, s := range r.servers {
		proxies = append(proxies, s)
	}
	r.mu.RUnlock()

	var cwg sync.WaitGroup
	for _, s := range proxies {
		cwg.Add(1)
		go func(sp *ServerProxy) {
			defer cwg.Done()
			_ = sp.Close()
		}(s)
	}
	closed := make(chan struct{})
	go func() { cwg.Wait(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
	}

	// Wait for all watcher goroutines to drain. They exit either on
	// r.done (closed above) or on their transport's Done (closed by
	// Close above), so this returns promptly.
	watchers := make(chan struct{})
	go func() { r.wg.Wait(); close(watchers) }()
	select {
	case <-watchers:
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

// Snapshots returns a deterministic, read-only view of every registered
// server's status. The returned slice is sorted by server name and each
// Tools list is sorted by tool name so callers get stable ordering
// without holding the registry lock. All slice fields are deep-copied.
func (r *Registry) Snapshots() []ServerSnapshot {
	if r == nil {
		return []ServerSnapshot{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ServerSnapshot, 0, len(r.servers))
	for _, p := range r.servers {
		var names []string
		for _, t := range p.Tools {
			// Apply the same filter as Tools().
			if p.enabledTools != nil {
				if !p.enabledTools[t.Name] {
					continue
				}
			} else if p.disabledTools != nil {
				if p.disabledTools[t.Name] {
					continue
				}
			}
			names = append(names, t.Name)
		}
		sort.Strings(names)
		out = append(out, ServerSnapshot{
			Name:         p.Name,
			State:        p.State,
			ToolCount:    len(names),
			Tools:        names,
			BackoffUntil: p.backoffUntil,
			LastError:    p.lastError,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// prefixSep separates the server name from the tool name in the
// fully-qualified MCP tool name: mcp__<server>__<tool>.
const mcpPrefix = "mcp__"

// Tools returns all MCP tools with their names prefixed as
// "mcp__<server>__<tool>". Per-server tool filters (enabled_tools,
// disabled_tools) are applied before returning.
func (r *Registry) Tools() []McpToolMeta {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []McpToolMeta
	for _, s := range r.servers {
		if s.State != StateConnected {
			continue
		}
		for _, t := range s.Tools {
			// Apply per-server filter.
			if s.enabledTools != nil {
				// Allowlist: only listed tools are exposed.
				if !s.enabledTools[t.Name] {
					continue
				}
			} else if s.disabledTools != nil {
				// Blocklist: listed tools are hidden.
				if s.disabledTools[t.Name] {
					continue
				}
			}
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

// SetToolFilter configures per-server tool filtering. enabledTools is an
// allowlist (only listed tools are exposed); disabledTools is a blocklist
// (listed tools are hidden). At most one may be non-nil; the caller
// (typically main.go) is responsible for ensuring they are not both set,
// since config validation already rejects that combination.
func (r *Registry) SetToolFilter(name string, enabledTools, disabledTools []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	proxy, ok := r.servers[name]
	if !ok {
		return
	}
	if len(enabledTools) > 0 {
		m := make(map[string]bool, len(enabledTools))
		for _, t := range enabledTools {
			m[t] = true
		}
		proxy.enabledTools = m
		proxy.disabledTools = nil
	} else if len(disabledTools) > 0 {
		m := make(map[string]bool, len(disabledTools))
		for _, t := range disabledTools {
			m[t] = true
		}
		proxy.disabledTools = m
		proxy.enabledTools = nil
	} else {
		proxy.enabledTools = nil
		proxy.disabledTools = nil
	}
}

// MCPChange describes a tool-level mutation detected between two schema snapshots.
type MCPChange struct {
	Kind     string // "tool_added", "tool_removed", "tool_schema_changed"
	ToolName string
}

// NOTE: SchemaHash was removed in M3 (docs/adr/0001). It hashed InputSchema raw
// (no key-sort), so a reordered-keys reconnect produced phantom drift in the
// epoch hash. MCP schema identity is now compared canonically via
// CompareToolLists / PendingSchemaChanges, and MCP tools that are actually sent
// to the model are already part of the canonical Prefix Fingerprint.

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

// transportFor returns a Transport for the given kind. It is the single
// dispatch point for transport construction so both Connect paths and tests
// exercise the same selection logic.
//   - "" or "stdio": NewStdioTransport (spawns command).
//   - "sse": NewSSETransport (SSE stream; caller must call Start).
//   - "http": NewHTTPTransport (plain HTTP JSON-RPC).
//   - "streamable-http": NewStreamableHTTPTransport (MCP spec 2025-03-26).
func transportFor(kind, url, command string, args []string, env map[string]string) (Transport, error) {
	switch kind {
	case "", "stdio":
		return NewStdioTransport(context.Background(), command, args, env)
	case "sse":
		return NewSSETransport(sseURLFromServerURL(url)), nil
	case "http":
		return NewHTTPTransport(url), nil
	case "streamable-http":
		return NewStreamableHTTPTransport(url), nil
	default:
		return nil, fmt.Errorf("unknown mcp transport %q", kind)
	}
}
