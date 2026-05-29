package mcp

import (
	"context"
	"encoding/json"
	"io"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// livePipe is a test harness for a single mock MCP server reachable over
// an in-process pipe. killServer EOFs the client's reader (by closing the
// server->client write end), which fires the transport's readerDone and
// drives the Registry watcher's degrade path — simulating process death.
type livePipe struct {
	transport *StdioTransport
	sw        io.WriteCloser // server's write end (close to EOF the client reader)
	closeAll  func()
}

// newLivePipe wires a mockMCPServer behind a fresh StdioTransport and
// returns it ready to be installed into a Registry. The transport is
// already initialized and its tools listed by the caller via dial-style
// helpers; here we only build the wire.
func newLivePipe(t *testing.T) *livePipe {
	t.Helper()
	sr, cw := io.Pipe() // server reads from sr, client writes to cw
	cr, sw := io.Pipe() // client reads from cr, server writes to sw
	done := make(chan struct{})

	go mockMCPServer(t, sr, sw, done)

	tr := newPipeTransport(cr, cw)
	lp := &livePipe{transport: tr, sw: sw}
	var once sync.Once
	lp.closeAll = func() {
		once.Do(func() {
			close(done)
			cw.Close()
			sw.Close()
			sr.Close()
			cr.Close()
		})
	}
	return lp
}

// killServer simulates the server process dying: closing the server's
// write end EOFs the client's reader, which triggers readLoop's
// defer close(readerDone) — the watcher's liveness signal.
func (lp *livePipe) killServer() {
	lp.sw.Close()
}

// connectVia installs a connected proxy backed by lp into reg and spawns
// the liveness watcher, exactly as Connect would, but using the in-process
// pipe transport. command is recorded so attemptReconnect has inputs.
func connectVia(t *testing.T, reg *Registry, name string, lp *livePipe) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	caps, err := initialize(ctx, lp.transport)
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	tl, err := listTools(ctx, lp.transport)
	if err != nil {
		t.Fatalf("listTools: %v", err)
	}

	reg.mu.Lock()
	reg.servers[name] = &ServerProxy{
		Name:    name,
		State:   StateConnected,
		Caps:    caps,
		Tools:   tl,
		t:       lp.transport,
		command: "mock", // non-empty so attemptReconnect doesn't short-circuit
	}
	reg.mu.Unlock()

	reg.wg.Add(1)
	go reg.watch(name, lp.transport.Done())
}

// stateOf returns the current lifecycle state of the named server.
func stateOf(reg *Registry, name string) LifecycleState {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	if s, ok := reg.servers[name]; ok {
		return s.State
	}
	return LifecycleState(-1)
}

// waitForState polls until the named server reaches want or the deadline
// elapses, returning whether it was reached.
func waitForState(reg *Registry, name string, want LifecycleState, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if stateOf(reg, name) == want {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return stateOf(reg, name) == want
}

// hasTool reports whether Tools() advertises any tool prefixed for srv.
func hasTool(reg *Registry, srv string) bool {
	for _, tl := range reg.Tools() {
		if strings.HasPrefix(tl.Name, mcpPrefix+srv+"__") {
			return true
		}
	}
	return false
}

// (a) process exit -> StateDegraded and Tools() drops the server's tools.
func TestProcessExitDegradesAndDropsTools(t *testing.T) {
	reg := NewRegistry()
	defer reg.Shutdown()

	lp := newLivePipe(t)
	defer lp.closeAll()
	connectVia(t, reg, "srv", lp)

	if stateOf(reg, "srv") != StateConnected {
		t.Fatalf("want StateConnected initially, got %v", stateOf(reg, "srv"))
	}
	if !hasTool(reg, "srv") {
		t.Fatal("Tools() should advertise srv tools while connected")
	}

	// Simulate process death.
	lp.killServer()

	if !waitForState(reg, "srv", StateDegraded, 2*time.Second) {
		t.Fatalf("server did not degrade, state=%v", stateOf(reg, "srv"))
	}
	if hasTool(reg, "srv") {
		t.Error("Tools() must drop degraded server's tools")
	}
	// CallTool must report not connected.
	_, _, err := reg.CallTool(context.Background(), "mcp__srv__echo", json.RawMessage(`{"text":"x"}`))
	if err == nil {
		t.Error("CallTool on degraded server should fail with not-connected")
	}
}

// (b) successful single reconnect -> back to StateConnected, tools re-listed,
// dialer invoked exactly once (the reconnect) on top of the manual connect.
func TestSingleReconnectSucceeds(t *testing.T) {
	reg := NewRegistry()
	reg.reconnectTimeout = 2 * time.Second
	defer reg.Shutdown()

	var mu sync.Mutex
	var dialCount int
	var reconnectPipe *livePipe

	// Inject a dialer whose reconnect invocation returns a fresh live pipe.
	reg.dialer = func(ctx context.Context, name, command string, args []string, env map[string]string) (dialResult, error) {
		lp := newLivePipe(t)
		mu.Lock()
		dialCount++
		reconnectPipe = lp
		mu.Unlock()
		caps, err := initialize(ctx, lp.transport)
		if err != nil {
			return dialResult{}, err
		}
		tl, err := listTools(ctx, lp.transport)
		if err != nil {
			return dialResult{}, err
		}
		return dialResult{t: lp.transport, caps: caps, tools: tl}, nil
	}

	// First (manual) connection.
	first := newLivePipe(t)
	defer first.closeAll()
	connectVia(t, reg, "srv", first)

	// Kill the first transport; the watcher should degrade then reconnect.
	// We poll for the dialer to have run (count==1) AND the state to be
	// StateConnected so we don't observe the pre-death StateConnected.
	first.killServer()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		c := dialCount
		mu.Unlock()
		if c == 1 && stateOf(reg, "srv") == StateConnected {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if stateOf(reg, "srv") != StateConnected {
		t.Fatalf("server did not reconnect, state=%v", stateOf(reg, "srv"))
	}
	if !hasTool(reg, "srv") {
		t.Error("Tools() should re-advertise srv tools after reconnect")
	}
	mu.Lock()
	got := dialCount
	mu.Unlock()
	if got != 1 {
		t.Errorf("dialer should be invoked exactly once for the reconnect, got %d", got)
	}
	mu.Lock()
	rp := reconnectPipe
	mu.Unlock()
	if rp != nil {
		defer rp.closeAll()
	}
}

// (c) reconnect failure sets backoffUntil and a second exit within the
// window does not re-dial.
func TestReconnectFailureSetsBackoffAndDoesNotRedial(t *testing.T) {
	reg := NewRegistry()
	reg.reconnectTimeout = 500 * time.Millisecond
	reg.reconnectBackoff = 30 * time.Second
	defer reg.Shutdown()

	var mu sync.Mutex
	var dialCount int

	reg.dialer = func(ctx context.Context, name, command string, args []string, env map[string]string) (dialResult, error) {
		mu.Lock()
		dialCount++
		mu.Unlock()
		return dialResult{}, io.ErrUnexpectedEOF
	}

	first := newLivePipe(t)
	defer first.closeAll()
	connectVia(t, reg, "srv", first)

	first.killServer()

	// After death + failed reconnect: degraded, reconnectAttempted=true,
	// backoffUntil in the future.
	if !waitForState(reg, "srv", StateDegraded, 2*time.Second) {
		t.Fatalf("server did not stay degraded, state=%v", stateOf(reg, "srv"))
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		reg.mu.RLock()
		p := reg.servers["srv"]
		attempted := p.reconnectAttempted
		future := time.Now().Before(p.backoffUntil)
		reg.mu.RUnlock()
		if attempted && future {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	reg.mu.RLock()
	p := reg.servers["srv"]
	if !p.reconnectAttempted {
		t.Error("reconnectAttempted should be true after failed reconnect")
	}
	if !time.Now().Before(p.backoffUntil) {
		t.Error("backoffUntil should be in the future after failed reconnect")
	}
	reg.mu.RUnlock()

	mu.Lock()
	afterFirst := dialCount
	mu.Unlock()
	if afterFirst != 1 {
		t.Fatalf("expected exactly 1 dial attempt, got %d", afterFirst)
	}

	// Directly invoking attemptReconnect again within the cooldown must
	// be a no-op (no third dial). The watcher already exited, so we drive
	// the gate directly to prove the backoff blocks re-entry.
	reg.attemptReconnect("srv")
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	afterSecond := dialCount
	mu.Unlock()
	if afterSecond != 1 {
		t.Errorf("flapping server must not re-dial within backoff: dialCount=%d", afterSecond)
	}
}

// (d) Shutdown after a degrade leaves zero leaked watcher goroutines.
//
// The pipe harness spawns its own mock-server and readLoop goroutines;
// those are freed by closeAll, not by Shutdown. To isolate the WATCHER
// goroutines (the only ones this stage adds and that Shutdown is
// responsible for reaping) we sample the baseline AFTER the harness
// goroutines exist and BEFORE any watcher is spawned, then assert the
// count returns to that baseline once Shutdown completes and the pipes
// are still open (so the harness goroutines are unchanged across the
// measurement window).
func TestShutdownStopsWatchersNoLeak(t *testing.T) {
	reg := NewRegistry()

	a := newLivePipe(t)
	defer a.closeAll()
	b := newLivePipe(t)
	defer b.closeAll()

	// Initialize/list the transports up front so the harness goroutines
	// (mockMCPServer + readLoop for each pipe) are all running before we
	// sample the baseline. We install proxies WITHOUT a watcher first.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for name, lp := range map[string]*livePipe{"a": a, "b": b} {
		caps, err := initialize(ctx, lp.transport)
		if err != nil {
			t.Fatalf("initialize %s: %v", name, err)
		}
		tl, err := listTools(ctx, lp.transport)
		if err != nil {
			t.Fatalf("listTools %s: %v", name, err)
		}
		reg.mu.Lock()
		reg.servers[name] = &ServerProxy{Name: name, State: StateConnected, Caps: caps, Tools: tl, t: lp.transport, command: "mock"}
		reg.mu.Unlock()
	}

	// Baseline: harness goroutines exist, no watchers yet.
	time.Sleep(50 * time.Millisecond)
	runtime.GC()
	before := runtime.NumGoroutine()

	// Now spawn the two watchers, exactly as Connect would.
	reg.wg.Add(1)
	go reg.watch("a", a.transport.Done())
	reg.wg.Add(1)
	go reg.watch("b", b.transport.Done())

	// Degrade one of them via process death.
	a.killServer()
	if !waitForState(reg, "a", StateDegraded, 2*time.Second) {
		t.Fatalf("server a did not degrade, state=%v", stateOf(reg, "a"))
	}

	done := make(chan struct{})
	go func() { reg.Shutdown(); close(done) }()
	select {
	case <-done:
	case <-time.After(6 * time.Second):
		t.Fatal("Shutdown did not return within deadline")
	}

	// After Shutdown, both watcher goroutines must be gone. The harness
	// goroutines are still alive (pipes not yet closed), so the count
	// must return to the pre-watcher baseline. Note: killing a's pipe
	// EOFs a's readLoop, which may also exit — that can only DECREASE the
	// count, so <= before is the correct assertion.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		runtime.GC()
		if runtime.NumGoroutine() <= before {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	runtime.GC()
	after := runtime.NumGoroutine()
	if after > before {
		t.Errorf("watcher goroutine leak after Shutdown: before(no watchers)=%d after=%d", before, after)
	}
}

// ADR-0001 at the mcp layer: degrade drops the server from Tools(), and
// CompareToolLists / PendingSchemaChanges reports the removal as a
// Capability-Set delta (not live fingerprint movement). The agent's
// frozen epoch keeps serving the old tool spec; here we assert only the
// mcp-layer half of that invariant.
func TestDegradeSurfacesAsCapabilitySetRemoval(t *testing.T) {
	reg := NewRegistry()
	defer reg.Shutdown()

	lp := newLivePipe(t)
	defer lp.closeAll()
	connectVia(t, reg, "srv", lp)

	// Snapshot the tool list while connected (this is what the frozen
	// epoch's FrozenTools would hold).
	frozen := reg.Tools()
	if len(frozen) == 0 {
		t.Fatal("expected tools while connected")
	}

	lp.killServer()
	if !waitForState(reg, "srv", StateDegraded, 2*time.Second) {
		t.Fatalf("server did not degrade, state=%v", stateOf(reg, "srv"))
	}

	// CompareToolLists(frozen, live) must report removal — the canonical
	// path CapabilityDiff uses.
	report := CompareToolLists(frozen, reg.Tools())
	if report.Kind != DriftRemoved {
		t.Errorf("expected DriftRemoved after degrade, got %s: %s", report.Kind, report.Message)
	}

	// PendingSchemaChanges (the other canonical comparator) must emit a
	// tool_removed for each dropped tool.
	changes := reg.PendingSchemaChanges(frozen)
	if len(changes) == 0 {
		t.Fatal("expected pending tool_removed changes after degrade")
	}
	for _, c := range changes {
		if c.Kind != "tool_removed" {
			t.Errorf("expected tool_removed, got %s for %s", c.Kind, c.ToolName)
		}
	}
}
