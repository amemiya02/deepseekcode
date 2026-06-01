package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/tools"
)

// mockMCPServer reads JSON-RPC from r and writes responses to w.
// It handles initialize, tools/list, and tools/call for "echo" and "add".
func mockMCPServer(t *testing.T, r io.Reader, w io.Writer, done <-chan struct{}) {
	t.Helper()
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	write := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintln(w, string(b))
	}

	for scanner.Scan() {
		select {
		case <-done:
			return
		default:
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		// Try request, then notification.
		var req jsonRPCRequest
		if err := json.Unmarshal(line, &req); err != nil || req.Method == "" {
			continue
		}

		switch req.Method {
		case "initialize":
			write(jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  json.RawMessage(`{"protocolVersion":"2025-06-18","capabilities":{"tools":{}},"serverInfo":{"name":"mock","version":"0.1"}}`),
			})
		case "tools/list":
			write(jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  json.RawMessage(`{"tools":[{"name":"echo","description":"echoes","inputSchema":{"type":"object","properties":{"text":{"type":"string"}}}},{"name":"add","description":"adds numbers","inputSchema":{"type":"object","properties":{"a":{"type":"number"},"b":{"type":"number"}}}}]}`),
			})
		case "tools/call":
			raw, _ := json.Marshal(req.Params)
			var p struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			json.Unmarshal(raw, &p)
			switch p.Name {
			case "echo":
				var a struct {
					Text string `json:"text"`
				}
				json.Unmarshal(p.Arguments, &a)
				write(jsonRPCResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Result:  json.RawMessage(fmt.Sprintf(`{"content":[{"type":"text","text":"echo: %s"}]}`, a.Text)),
				})
			case "add":
				var a struct {
					A int `json:"a"`
					B int `json:"b"`
				}
				json.Unmarshal(p.Arguments, &a)
				write(jsonRPCResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Result:  json.RawMessage(fmt.Sprintf(`{"content":[{"type":"text","text":"%d"}]}`, a.A+a.B)),
				})
			default:
				write(jsonRPCResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Error:   &jsonRPCError{Code: -32601, Message: "unknown tool"},
				})
			}
		default:
			write(jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &jsonRPCError{Code: -32601, Message: "method not found"},
			})
		}
	}
}

// pipePair returns connected read/write pipe ends for an in-process
// mock MCP transport. The serverFn runs in a goroutine consuming from
// sr and writing to sw.
func pipePair(t *testing.T, serverFn func(io.Reader, io.Writer, <-chan struct{})) (*StdioTransport, func()) {
	t.Helper()
	sr, cw := io.Pipe() // server reads from sr, client writes to cw
	cr, sw := io.Pipe() // client reads from cr, server writes to sw
	done := make(chan struct{})

	go serverFn(sr, sw, done)

	tr := newPipeTransport(cr, cw)
	cleanup := func() {
		close(done)
		cw.Close()
		sw.Close()
		sr.Close()
		cr.Close()
	}
	return tr, cleanup
}

// ---------- tests ----------

func TestStdioTransportRoundtrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tr, cleanup := pipePair(t, func(r io.Reader, w io.Writer, done <-chan struct{}) {
		mockMCPServer(t, r, w, done)
	})
	defer cleanup()

	raw, err := tr.Send(ctx, "tools/list", nil)
	if err != nil {
		t.Fatalf("Send tools/list: %v", err)
	}
	var result struct {
		Tools []McpToolMeta `json:"tools"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("unmarshal tools/list: %v", err)
	}
	if len(result.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(result.Tools))
	}
	if result.Tools[0].Name != "echo" {
		t.Errorf("tool[0].name = %q, want echo", result.Tools[0].Name)
	}

	// Notification should not block.
	if err := tr.Notify(ctx, "notifications/initialized", nil); err != nil {
		t.Errorf("Notify: %v", err)
	}
}

func TestInitializeHandshake(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tr, cleanup := pipePair(t, func(r io.Reader, w io.Writer, done <-chan struct{}) {
		mockMCPServer(t, r, w, done)
	})
	defer cleanup()

	caps, err := initialize(ctx, tr)
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if !caps.Tools {
		t.Error("expected Tools capability")
	}
}

func TestListAndCallTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tr, cleanup := pipePair(t, func(r io.Reader, w io.Writer, done <-chan struct{}) {
		mockMCPServer(t, r, w, done)
	})
	defer cleanup()

	if _, err := initialize(ctx, tr); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	tools, err := listTools(ctx, tr)
	if err != nil {
		t.Fatalf("listTools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	content, isError, err := callTool(ctx, tr, "echo", json.RawMessage(`{"text":"hello"}`))
	if err != nil {
		t.Fatalf("callTool echo: %v", err)
	}
	if isError {
		t.Error("expected isError=false")
	}
	if !strings.Contains(content, "echo: hello") {
		t.Errorf("content = %q, want substring %q", content, "echo: hello")
	}

	content, isError, err = callTool(ctx, tr, "add", json.RawMessage(`{"a":3,"b":4}`))
	if err != nil {
		t.Fatalf("callTool add: %v", err)
	}
	if isError {
		t.Error("expected isError=false")
	}
	if !strings.Contains(content, "7") {
		t.Errorf("content = %q, want substring %q", content, "7")
	}
}

func TestRegistryConnectAndCall(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	reg := NewRegistry()

	// Manually set up a connected server via pipe transport.
	tr, cleanup := pipePair(t, func(r io.Reader, w io.Writer, done <-chan struct{}) {
		mockMCPServer(t, r, w, done)
	})
	defer cleanup()

	caps, err := initialize(ctx, tr)
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	tools, err := listTools(ctx, tr)
	if err != nil {
		t.Fatalf("listTools: %v", err)
	}

	reg.mu.Lock()
	reg.servers["mock"] = &ServerProxy{
		Name:  "mock",
		State: StateConnected,
		Caps:  caps,
		Tools: tools,
		t:     tr,
	}
	reg.mu.Unlock()

	// Call via registry.
	content, isError, err := reg.CallTool(ctx, "mcp__mock__echo", json.RawMessage(`{"text":"hi"}`))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if isError {
		t.Error("expected isError=false")
	}

	// Unknown server.
	_, _, err = reg.CallTool(ctx, "mcp__nonexistent__tool", nil)
	if err == nil {
		t.Error("expected error for unknown server")
	}

	// Bad prefix.
	_, _, err = reg.CallTool(ctx, "not_an_mcp_tool", nil)
	if err == nil {
		t.Error("expected error for bad prefix")
	}

	if content == "" {
		t.Error("expected non-empty content from echo tool")
	}
}

func TestBridgeAll(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	reg := NewRegistry()

	tr, cleanup := pipePair(t, func(r io.Reader, w io.Writer, done <-chan struct{}) {
		mockMCPServer(t, r, w, done)
	})
	defer cleanup()

	caps, _ := initialize(ctx, tr)
	tl, _ := listTools(ctx, tr)

	reg.mu.Lock()
	reg.servers["mock"] = &ServerProxy{
		Name:  "mock",
		State: StateConnected,
		Caps:  caps,
		Tools: tl,
		t:     tr,
	}
	reg.mu.Unlock()

	bridged := BridgeAll(reg)
	if len(bridged) != 2 {
		t.Fatalf("expected 2 bridged tools, got %d", len(bridged))
	}
	byName := map[string]tools.Tool{}
	for _, tl := range bridged {
		byName[tl.Name()] = tl
	}
	addTool, ok := byName["mcp__mock__add"]
	if !ok {
		t.Fatal("missing mcp__mock__add")
	}
	echoTool, ok := byName["mcp__mock__echo"]
	if !ok {
		t.Fatal("missing mcp__mock__echo")
	}
	_ = echoTool

	// Execute a bridged tool through the tools.Tool interface.
	res, err := addTool.Execute(ctx, json.RawMessage(`{"a":10,"b":20}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(res.Content, "30") {
		t.Errorf("content = %q, want 30", res.Content)
	}
}

func TestRegistryTimeout(t *testing.T) {
	ctx := context.Background()

	reg := NewRegistry()

	// Create a transport whose server never responds to "slow" tool.
	tr, cleanup := pipePair(t, func(r io.Reader, w io.Writer, done <-chan struct{}) {
		// Only respond to initialize, nothing else — causes Send to block.
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			select {
			case <-done:
				return
			default:
			}
			var req jsonRPCRequest
			if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
				continue
			}
			if req.Method == "initialize" {
				b, _ := json.Marshal(jsonRPCResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Result:  json.RawMessage(`{"protocolVersion":"2025-06-18","capabilities":{"tools":{}},"serverInfo":{"name":"slow","version":"0.1"}}`),
				})
				fmt.Fprintln(w, string(b))
			}
			// Don't respond to other methods.
		}
	})
	defer cleanup()

	caps, _ := initialize(ctx, tr)

	reg.mu.Lock()
	reg.servers["slow"] = &ServerProxy{
		Name:  "slow",
		State: StateConnected,
		Caps:  caps,
		Tools: []McpToolMeta{{Name: "slow", Description: "never responds", InputSchema: json.RawMessage(`{}`)}},
		t:     tr,
	}
	reg.mu.Unlock()

	// 50ms timeout should trigger before the server responds.
	reg.SetTimeout("slow", 0) // 0 → use context only
	shortCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, _, err := reg.CallTool(shortCtx, "mcp__slow__slow", json.RawMessage(`{}`))
	if err == nil {
		t.Error("expected timeout error from non-responding server")
	}
}

func TestToolsInterface(t *testing.T) {
	// Verify mcpAdaptedTool satisfies tools.Tool.
	var _ tools.Tool = (*mcpAdaptedTool)(nil)
}

func TestShutdown(t *testing.T) {
	reg := NewRegistry()

	tr, cleanup := pipePair(t, func(r io.Reader, w io.Writer, done <-chan struct{}) {
		mockMCPServer(t, r, w, done)
	})
	defer cleanup()

	reg.mu.Lock()
	reg.servers["test"] = &ServerProxy{
		Name:  "test",
		State: StateConnected,
		t:     tr,
	}
	reg.mu.Unlock()

	// Shutdown should not panic even with pipe transport.
	reg.Shutdown()
}

func TestLifecycleStateString(t *testing.T) {
	cases := []struct {
		state LifecycleState
		want  string
	}{
		{StateInitializing, "initializing"},
		{StateConnected, "connected"},
		{StateDegraded, "degraded"},
		{StateFailed, "failed"},
		{LifecycleState(99), "unknown"},
	}
	for _, c := range cases {
		if got := c.state.String(); got != c.want {
			t.Errorf("%d.String() = %q, want %q", c.state, got, c.want)
		}
	}
}

func TestSendCancelledContext(t *testing.T) {
	// Use a server that reads but never responds, so the Send write
	// succeeds but the read blocks until ctx cancellation.
	done := make(chan struct{})
	sr, cw := io.Pipe()
	cr, sw := io.Pipe()
	go func() {
		scanner := bufio.NewScanner(sr)
		for scanner.Scan() {
			// Read and ignore — never respond.
		}
	}()
	tr := newPipeTransport(cr, cw)
	_ = sw
	defer func() {
		close(done)
		cw.Close()
		cr.Close()
		sr.Close()
		sw.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := tr.Send(ctx, "tools/list", nil)
	if err == nil {
		t.Error("expected timeout/cancellation error")
	}
}

func TestCallToolUnknownServer(t *testing.T) {
	reg := NewRegistry()
	_, _, err := reg.CallTool(context.Background(), "mcp__no__tool", nil)
	if err == nil {
		t.Error("expected error for unregistered server")
	}
}

func TestCallToolBadPrefix(t *testing.T) {
	reg := NewRegistry()
	_, _, err := reg.CallTool(context.Background(), "bash", nil)
	if err == nil {
		t.Error("expected error for non-mcp tool name")
	}
}

func TestCallToolNoDoubleSep(t *testing.T) {
	reg := NewRegistry()
	_, _, err := reg.CallTool(context.Background(), "mcp__nodouble", nil)
	if err == nil {
		t.Error("expected error for tool name missing __ separator")
	}
}

func TestRegistryDuplicateName(t *testing.T) {
	reg := NewRegistry()
	reg.mu.Lock()
	reg.servers["dup"] = &ServerProxy{Name: "dup", State: StateConnected}
	reg.mu.Unlock()
	// Connect with same name should fail.
	err := reg.Connect(context.Background(), "dup", "false", nil, nil)
	if err == nil {
		t.Error("expected duplicate name error")
	}
}

func TestNewStdioTransportSubprocessError(t *testing.T) {
	ctx := context.Background()
	_, err := NewStdioTransport(ctx, "/nonexistent/binary/xyzzy", nil, nil)
	if err == nil {
		t.Error("expected error for nonexistent binary")
	}
}

func TestRegistryConnectSubprocessError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	reg := NewRegistry()
	err := reg.Connect(ctx, "bad", "/nonexistent/binary/xyzzy", nil, nil)
	if err == nil {
		t.Error("expected error for bad binary")
	}
}

func TestRegistryServers(t *testing.T) {
	reg := NewRegistry()
	reg.mu.Lock()
	reg.servers["a"] = &ServerProxy{Name: "a", State: StateConnected}
	reg.servers["b"] = &ServerProxy{Name: "b", State: StateFailed}
	reg.mu.Unlock()

	srvs := reg.Servers()
	if len(srvs) != 2 {
		t.Errorf("expected 2 servers, got %d", len(srvs))
	}
}

func TestStdioTransportCloseIdempotent(t *testing.T) {
	tr, cleanup := pipePair(t, func(r io.Reader, w io.Writer, done <-chan struct{}) {
		mockMCPServer(t, r, w, done)
	})
	defer cleanup()

	if err := tr.Close(); err != nil {
		t.Logf("first Close: %v", err)
	}
	// Second Close should be a no-op (sync.Once).
	if err := tr.Close(); err != nil {
		t.Logf("second Close: %v", err)
	}
}

func TestRegistryFailoverPartial(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	reg := NewRegistry()

	// Server "good": connected with tools via pipe transport.
	tr, cleanup := pipePair(t, func(r io.Reader, w io.Writer, done <-chan struct{}) {
		mockMCPServer(t, r, w, done)
	})
	defer cleanup()

	caps, err := initialize(ctx, tr)
	if err != nil {
		t.Fatalf("initialize good: %v", err)
	}
	tools, err := listTools(ctx, tr)
	if err != nil {
		t.Fatalf("listTools good: %v", err)
	}

	reg.mu.Lock()
	reg.servers["good"] = &ServerProxy{
		Name:  "good",
		State: StateConnected,
		Caps:  caps,
		Tools: tools,
		t:     tr,
	}
	// Server "bad": failed state, no transport.
	reg.servers["bad"] = &ServerProxy{
		Name:  "bad",
		State: StateFailed,
	}
	reg.mu.Unlock()

	// Tools() should only include tools from "good".
	allTools := reg.Tools()
	for _, tl := range allTools {
		if strings.HasPrefix(tl.Name, "mcp__bad__") {
			t.Errorf("Tools() includes failed server tool: %s", tl.Name)
		}
	}
	found := false
	for _, tl := range allTools {
		if strings.HasPrefix(tl.Name, "mcp__good__") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Tools() missing tools from connected server")
	}

	// CallTool should work for "good".
	content, isError, err := reg.CallTool(ctx, "mcp__good__echo", json.RawMessage(`{"text":"failover"}`))
	if err != nil {
		t.Fatalf("CallTool good: %v", err)
	}
	if isError {
		t.Error("expected isError=false")
	}
	if content == "" {
		t.Error("expected non-empty content")
	}

	// CallTool should fail for "bad".
	_, _, err = reg.CallTool(ctx, "mcp__bad__echo", nil)
	if err == nil {
		t.Error("expected error for failed server")
	}
}

func TestConnectSSEViaSeam(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	reg := NewRegistry()

	// Set up SSE dialer seam that injects a mock pipe transport.
	tr, cleanup := pipePair(t, func(r io.Reader, w io.Writer, done <-chan struct{}) {
		mockMCPServer(t, r, w, done)
	})
	defer cleanup()

	caps, err := initialize(ctx, tr)
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	tools, err := listTools(ctx, tr)
	if err != nil {
		t.Fatalf("listTools: %v", err)
	}

	reg.sseDialer = func(ctx context.Context, name, sseURL string) (dialResult, error) {
		return dialResult{t: tr, caps: caps, tools: tools}, nil
	}

	if err := reg.ConnectSSE(ctx, "sse-mock", "https://example.com/sse"); err != nil {
		t.Fatalf("ConnectSSE: %v", err)
	}

	// Verify the server is connected with the SSE transport kind.
	reg.mu.RLock()
	proxy := reg.servers["sse-mock"]
	reg.mu.RUnlock()
	if proxy.State != StateConnected {
		t.Errorf("state = %v, want connected", proxy.State)
	}
	if proxy.kind != transportSSE {
		t.Errorf("kind = %v, want transportSSE", proxy.kind)
	}
	if proxy.sseURL != "https://example.com/sse" {
		t.Errorf("sseURL = %q, want https://example.com/sse", proxy.sseURL)
	}

	// Verify tool bridge works.
	bridged := BridgeAll(reg)
	if len(bridged) != 2 {
		t.Fatalf("expected 2 bridged tools, got %d", len(bridged))
	}

	// Verify call through registry.
	content, isError, err := reg.CallTool(ctx, "mcp__sse-mock__echo", json.RawMessage(`{"text":"hi"}`))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if isError {
		t.Error("expected isError=false")
	}
	if content == "" {
		t.Error("expected non-empty content from echo tool")
	}

	// Verify snapshot includes the SSE server.
	snaps := reg.Snapshots()
	var found bool
	for _, s := range snaps {
		if s.Name == "sse-mock" && s.State == StateConnected {
			found = true
		}
	}
	if !found {
		t.Error("SSE server not found in snapshots")
	}
}

func TestConnectSSEFailedConnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	reg := NewRegistry()
	reg.sseDialer = func(ctx context.Context, name, sseURL string) (dialResult, error) {
		return dialResult{}, fmt.Errorf("connection refused")
	}

	err := reg.ConnectSSE(ctx, "bad-sse", "https://example.com/sse")
	if err == nil {
		t.Fatal("expected error for failed SSE connect")
	}

	// Verify the server is in failed state.
	reg.mu.RLock()
	proxy := reg.servers["bad-sse"]
	reg.mu.RUnlock()
	if proxy.State != StateFailed {
		t.Errorf("state = %v, want failed", proxy.State)
	}
	if proxy.lastError == "" {
		t.Error("expected lastError to be set")
	}

	// Verify snapshot shows the failed server.
	snaps := reg.Snapshots()
	var found bool
	for _, s := range snaps {
		if s.Name == "bad-sse" && s.State == StateFailed {
			found = true
		}
	}
	if !found {
		t.Error("failed SSE server not found in snapshots")
	}
}

func TestStdioBackwardCompatWithNewConfig(t *testing.T) {
	// Verify that a config with Transport="" (default stdio) still works
	// with Connect, and that the transport kind is stored correctly.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	reg := NewRegistry()

	tr, cleanup := pipePair(t, func(r io.Reader, w io.Writer, done <-chan struct{}) {
		mockMCPServer(t, r, w, done)
	})
	defer cleanup()

	caps, err := initialize(ctx, tr)
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	tools, err := listTools(ctx, tr)
	if err != nil {
		t.Fatalf("listTools: %v", err)
	}

	reg.dialer = func(ctx context.Context, name, command string, args []string, env map[string]string) (dialResult, error) {
		return dialResult{t: tr, caps: caps, tools: tools}, nil
	}

	if err := reg.Connect(ctx, "local", "echo", nil, nil); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	reg.mu.RLock()
	proxy := reg.servers["local"]
	reg.mu.RUnlock()
	if proxy.kind != transportStdio {
		t.Errorf("kind = %v, want transportStdio", proxy.kind)
	}
	if proxy.sseURL != "" {
		t.Errorf("sseURL = %q, want empty for stdio", proxy.sseURL)
	}
}

// ---------- Task-015: Per-tool enable/disable ----------

func TestSetToolFilterDisabledTools(t *testing.T) {
	reg := NewRegistry()

	// Set up a mock server with 3 tools.
	reg.mu.Lock()
	reg.servers["fs"] = &ServerProxy{
		Name:  "fs",
		State: StateConnected,
		Tools: []McpToolMeta{
			{Name: "read_file", Description: "read", InputSchema: json.RawMessage(`{}`)},
			{Name: "write_file", Description: "write", InputSchema: json.RawMessage(`{}`)},
			{Name: "delete_file", Description: "delete", InputSchema: json.RawMessage(`{}`)},
		},
	}
	reg.mu.Unlock()

	// Disable delete_file.
	reg.SetToolFilter("fs", nil, []string{"delete_file"})

	tools := reg.Tools()
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools after disable, got %d", len(tools))
	}
	for _, tool := range tools {
		if strings.HasSuffix(tool.Name, "delete_file") {
			t.Errorf("delete_file should be hidden, got %q", tool.Name)
		}
	}

	// Snapshots should also reflect the filter.
	snaps := reg.Snapshots()
	for _, s := range snaps {
		if s.Name == "fs" {
			if s.ToolCount != 2 {
				t.Errorf("snapshot ToolCount = %d, want 2", s.ToolCount)
			}
			for _, n := range s.Tools {
				if n == "delete_file" {
					t.Error("delete_file should not appear in snapshot")
				}
			}
		}
	}
}

func TestSetToolFilterEnabledTools(t *testing.T) {
	reg := NewRegistry()

	reg.mu.Lock()
	reg.servers["github"] = &ServerProxy{
		Name:  "github",
		State: StateConnected,
		Tools: []McpToolMeta{
			{Name: "search_issues", Description: "search", InputSchema: json.RawMessage(`{}`)},
			{Name: "read_file", Description: "read", InputSchema: json.RawMessage(`{}`)},
			{Name: "create_issue", Description: "create", InputSchema: json.RawMessage(`{}`)},
		},
	}
	reg.mu.Unlock()

	// Enable only search_issues and read_file.
	reg.SetToolFilter("github", []string{"search_issues", "read_file"}, nil)

	tools := reg.Tools()
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools after enable, got %d", len(tools))
	}
	for _, tool := range tools {
		if strings.HasSuffix(tool.Name, "create_issue") {
			t.Errorf("create_issue should be hidden, got %q", tool.Name)
		}
	}
}

func TestSetToolFilterClearsPreviousFilter(t *testing.T) {
	reg := NewRegistry()

	reg.mu.Lock()
	reg.servers["fs"] = &ServerProxy{
		Name:  "fs",
		State: StateConnected,
		Tools: []McpToolMeta{
			{Name: "read_file", Description: "read", InputSchema: json.RawMessage(`{}`)},
			{Name: "write_file", Description: "write", InputSchema: json.RawMessage(`{}`)},
		},
	}
	reg.mu.Unlock()

	// Set a disabled filter, then clear it.
	reg.SetToolFilter("fs", nil, []string{"write_file"})
	if len(reg.Tools()) != 1 {
		t.Fatalf("expected 1 tool with disabled filter, got %d", len(reg.Tools()))
	}

	// Clear filter.
	reg.SetToolFilter("fs", nil, nil)
	if len(reg.Tools()) != 2 {
		t.Fatalf("expected 2 tools after clearing filter, got %d", len(reg.Tools()))
	}
}

func TestSetToolFilterEnabledWinsOverDisabled(t *testing.T) {
	reg := NewRegistry()

	reg.mu.Lock()
	reg.servers["fs"] = &ServerProxy{
		Name:  "fs",
		State: StateConnected,
		Tools: []McpToolMeta{
			{Name: "read_file", Description: "read", InputSchema: json.RawMessage(`{}`)},
			{Name: "write_file", Description: "write", InputSchema: json.RawMessage(`{}`)},
		},
	}
	reg.mu.Unlock()

	// SetToolFilter with enabledTools takes precedence.
	reg.SetToolFilter("fs", []string{"read_file"}, []string{"write_file"})

	tools := reg.Tools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool (enabled wins), got %d", len(tools))
	}
	if !strings.HasSuffix(tools[0].Name, "read_file") {
		t.Errorf("expected read_file, got %q", tools[0].Name)
	}
}

func TestSetToolFilterNonexistentServer(t *testing.T) {
	reg := NewRegistry()
	// Should not panic.
	reg.SetToolFilter("nonexistent", nil, []string{"foo"})
}

func TestToolFilterToolNameNotProvidedByServer(t *testing.T) {
	reg := NewRegistry()

	reg.mu.Lock()
	reg.servers["fs"] = &ServerProxy{
		Name:  "fs",
		State: StateConnected,
		Tools: []McpToolMeta{
			{Name: "read_file", Description: "read", InputSchema: json.RawMessage(`{}`)},
		},
	}
	reg.mu.Unlock()

	// Enable a tool that doesn't exist — should result in 0 tools.
	reg.SetToolFilter("fs", []string{"nonexistent_tool"}, nil)

	tools := reg.Tools()
	if len(tools) != 0 {
		t.Fatalf("expected 0 tools when enabling nonexistent tool, got %d", len(tools))
	}
}

func TestToolFilterRemovesAllTools(t *testing.T) {
	reg := NewRegistry()

	reg.mu.Lock()
	reg.servers["fs"] = &ServerProxy{
		Name:  "fs",
		State: StateConnected,
		Tools: []McpToolMeta{
			{Name: "read_file", Description: "read", InputSchema: json.RawMessage(`{}`)},
			{Name: "write_file", Description: "write", InputSchema: json.RawMessage(`{}`)},
		},
	}
	reg.mu.Unlock()

	// Disable all tools.
	reg.SetToolFilter("fs", nil, []string{"read_file", "write_file"})

	tools := reg.Tools()
	if len(tools) != 0 {
		t.Fatalf("expected 0 tools when all disabled, got %d", len(tools))
	}

	// BridgeAll should also return 0 tools for this server.
	bridged := BridgeAll(reg)
	if len(bridged) != 0 {
		t.Fatalf("expected 0 bridged tools, got %d", len(bridged))
	}
}

func TestToolFilterPendingSchemaChanges(t *testing.T) {
	reg := NewRegistry()

	reg.mu.Lock()
	reg.servers["fs"] = &ServerProxy{
		Name:  "fs",
		State: StateConnected,
		Tools: []McpToolMeta{
			{Name: "read_file", Description: "read", InputSchema: json.RawMessage(`{}`)},
			{Name: "write_file", Description: "write", InputSchema: json.RawMessage(`{}`)},
		},
	}
	reg.mu.Unlock()

	// Snapshot the current (unfiltered) tools.
	oldTools := reg.Tools()

	// Now disable write_file.
	reg.SetToolFilter("fs", nil, []string{"write_file"})

	// PendingSchemaChanges should detect the removal.
	changes := reg.PendingSchemaChanges(oldTools)
	var foundRemove bool
	for _, c := range changes {
		if c.Kind == "tool_removed" && strings.HasSuffix(c.ToolName, "write_file") {
			foundRemove = true
		}
	}
	if !foundRemove {
		t.Errorf("expected tool_removed for write_file, got changes: %v", changes)
	}
}
