package mcp

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"
)

func TestSnapshotsEmptyRegistry(t *testing.T) {
	reg := NewRegistry()
	snaps := reg.Snapshots()
	if snaps == nil {
		t.Fatal("Snapshots() must not return nil for empty registry")
	}
	if len(snaps) != 0 {
		t.Fatalf("expected 0 snapshots, got %d", len(snaps))
	}
}

func TestSnapshotsNilSafe(t *testing.T) {
	// A nil *Registry must return an empty non-nil slice, not panic.
	var reg *Registry
	snaps := reg.Snapshots()
	if snaps == nil {
		t.Fatal("nil registry Snapshots() returned nil, want non-nil empty slice")
	}
	if len(snaps) != 0 {
		t.Fatalf("nil registry Snapshots() = %d items, want 0", len(snaps))
	}
}

func TestSnapshotsConnected(t *testing.T) {
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
	tl, err := listTools(ctx, tr)
	if err != nil {
		t.Fatalf("listTools: %v", err)
	}

	reg.mu.Lock()
	reg.servers["mock"] = &ServerProxy{
		Name:  "mock",
		State: StateConnected,
		Caps:  caps,
		Tools: tl,
		t:     tr,
	}
	reg.mu.Unlock()

	snaps := reg.Snapshots()
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}
	s := snaps[0]
	if s.Name != "mock" {
		t.Errorf("Name = %q, want mock", s.Name)
	}
	if s.State != StateConnected {
		t.Errorf("State = %v, want StateConnected", s.State)
	}
	if s.ToolCount != 2 {
		t.Errorf("ToolCount = %d, want 2", s.ToolCount)
	}
	if len(s.Tools) != 2 {
		t.Fatalf("Tools len = %d, want 2", len(s.Tools))
	}
	// Tools must be sorted.
	if s.Tools[0] >= s.Tools[1] {
		t.Errorf("Tools not sorted: %v", s.Tools)
	}
	if s.LastError != "" {
		t.Errorf("LastError = %q, want empty", s.LastError)
	}
	if !s.BackoffUntil.IsZero() {
		t.Errorf("BackoffUntil should be zero for connected server")
	}
}

func TestSnapshotsFailedConnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	reg := NewRegistry()

	// Connect with a nonexistent binary to trigger StateFailed.
	err := reg.Connect(ctx, "bad", "/nonexistent/binary/xyzzy", nil, nil)
	if err == nil {
		t.Fatal("expected error for nonexistent binary")
	}

	snaps := reg.Snapshots()
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}
	s := snaps[0]
	if s.State != StateFailed {
		t.Errorf("State = %v, want StateFailed", s.State)
	}
	if s.LastError == "" {
		t.Error("LastError should be non-empty after failed connect")
	}
	if s.ToolCount != 0 {
		t.Errorf("ToolCount = %d, want 0", s.ToolCount)
	}
}

func TestSnapshotsDegradedWithFailedReconnect(t *testing.T) {
	reg := NewRegistry()
	reg.reconnectTimeout = 500 * time.Millisecond
	reg.reconnectBackoff = 30 * time.Second
	defer reg.Shutdown()

	// Dialer always fails.
	reg.dialer = func(ctx context.Context, name, command string, args []string, env map[string]string) (dialResult, error) {
		return dialResult{}, io.ErrUnexpectedEOF
	}

	lp := newLivePipe(t)
	defer lp.closeAll()
	connectVia(t, reg, "srv", lp)

	// Kill the server to trigger degrade + reconnect failure.
	lp.killServer()

	if !waitForState(reg, "srv", StateDegraded, 2*time.Second) {
		t.Fatalf("server did not degrade, state=%v", stateOf(reg, "srv"))
	}

	// Wait for reconnectAttempted to be set.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		reg.mu.RLock()
		p := reg.servers["srv"]
		attempted := p.reconnectAttempted
		reg.mu.RUnlock()
		if attempted {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	snaps := reg.Snapshots()
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}
	s := snaps[0]
	if s.State != StateDegraded {
		t.Errorf("State = %v, want StateDegraded", s.State)
	}
	if s.LastError == "" {
		t.Error("LastError should be non-empty after failed reconnect")
	}
	if s.BackoffUntil.IsZero() {
		t.Error("BackoffUntil should be non-zero after failed reconnect")
	}
	if !time.Now().Before(s.BackoffUntil) {
		t.Error("BackoffUntil should be in the future")
	}
	if s.ToolCount != 0 {
		t.Errorf("ToolCount = %d, want 0 for degraded server", s.ToolCount)
	}
}

func TestSnapshotsDeterministicOrder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	reg := NewRegistry()

	tr1, cleanup1 := pipePair(t, func(r io.Reader, w io.Writer, done <-chan struct{}) {
		mockMCPServer(t, r, w, done)
	})
	defer cleanup1()
	tr2, cleanup2 := pipePair(t, func(r io.Reader, w io.Writer, done <-chan struct{}) {
		mockMCPServer(t, r, w, done)
	})
	defer cleanup2()

	caps1, _ := initialize(ctx, tr1)
	tl1, _ := listTools(ctx, tr1)
	caps2, _ := initialize(ctx, tr2)
	tl2, _ := listTools(ctx, tr2)

	// Insert in reverse alphabetical order.
	reg.mu.Lock()
	reg.servers["zebra"] = &ServerProxy{Name: "zebra", State: StateConnected, Caps: caps1, Tools: tl1, t: tr1}
	reg.servers["alpha"] = &ServerProxy{Name: "alpha", State: StateConnected, Caps: caps2, Tools: tl2, t: tr2}
	reg.servers["middle"] = &ServerProxy{Name: "middle", State: StateFailed}
	reg.mu.Unlock()

	snaps := reg.Snapshots()
	if len(snaps) != 3 {
		t.Fatalf("expected 3 snapshots, got %d", len(snaps))
	}
	// Must be sorted by name.
	if snaps[0].Name != "alpha" {
		t.Errorf("snap[0].Name = %q, want alpha", snaps[0].Name)
	}
	if snaps[1].Name != "middle" {
		t.Errorf("snap[1].Name = %q, want middle", snaps[1].Name)
	}
	if snaps[2].Name != "zebra" {
		t.Errorf("snap[2].Name = %q, want zebra", snaps[2].Name)
	}

	// Call Snapshots again — order must be stable.
	snaps2 := reg.Snapshots()
	for i := range snaps {
		if snaps[i].Name != snaps2[i].Name {
			t.Errorf("iteration %d: order changed from %q to %q", i, snaps[i].Name, snaps2[i].Name)
		}
	}
}

func TestSnapshotsToolNamesCopied(t *testing.T) {
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
	reg.servers["mock"] = &ServerProxy{Name: "mock", State: StateConnected, Caps: caps, Tools: tl, t: tr}
	reg.mu.Unlock()

	snaps := reg.Snapshots()
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}

	originalLen := len(snaps[0].Tools)
	// Mutate the returned slice — must not affect registry state.
	snaps[0].Tools[0] = "MUTATED"

	snaps2 := reg.Snapshots()
	if snaps2[0].Tools[0] == "MUTATED" {
		t.Error("Snapshot tool names were not deep-copied; mutation leaked into registry")
	}
	if len(snaps2[0].Tools) != originalLen {
		t.Errorf("Tool count changed after mutation: %d vs %d", len(snaps2[0].Tools), originalLen)
	}
}

func TestSnapshotMultipleStates(t *testing.T) {
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
	reg.servers["connected"] = &ServerProxy{Name: "connected", State: StateConnected, Caps: caps, Tools: tl, t: tr}
	reg.servers["degraded"] = &ServerProxy{Name: "degraded", State: StateDegraded, lastError: "process died"}
	reg.servers["failed"] = &ServerProxy{Name: "failed", State: StateFailed, lastError: "spawn failed"}
	reg.servers["initializing"] = &ServerProxy{Name: "initializing", State: StateInitializing}
	reg.mu.Unlock()

	snaps := reg.Snapshots()
	if len(snaps) != 4 {
		t.Fatalf("expected 4 snapshots, got %d", len(snaps))
	}

	byName := make(map[string]ServerSnapshot)
	for _, s := range snaps {
		byName[s.Name] = s
	}

	if byName["connected"].State != StateConnected {
		t.Error("connected snapshot has wrong state")
	}
	if byName["connected"].ToolCount != 2 {
		t.Error("connected snapshot should have 2 tools")
	}
	if byName["degraded"].LastError != "process died" {
		t.Errorf("degraded LastError = %q, want %q", byName["degraded"].LastError, "process died")
	}
	if byName["failed"].LastError != "spawn failed" {
		t.Errorf("failed LastError = %q, want %q", byName["failed"].LastError, "spawn failed")
	}
	if byName["initializing"].LastError != "" {
		t.Error("initializing snapshot should have empty LastError")
	}
}

func TestSnapshotsMcpToolMetaUntouched(t *testing.T) {
	// Ensure McpToolMeta JSON roundtrip still works (server.go changes
	// did not break the struct).
	raw := `{"name":"echo","description":"echoes","inputSchema":{"type":"object"}}`
	var m McpToolMeta
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("unmarshal McpToolMeta: %v", err)
	}
	if m.Name != "echo" || m.Description != "echoes" {
		t.Errorf("McpToolMeta roundtrip failed: %+v", m)
	}
}
