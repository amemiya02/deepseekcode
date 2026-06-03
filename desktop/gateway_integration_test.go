package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/gateway"
)

// freePort returns an OS-assigned free TCP port on loopback.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// waitForServer polls url until it responds or the deadline elapses.
func waitForServer(t *testing.T, url string, timeout time.Duration) *http.Response {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			return resp
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("server at %s did not come up within %s", url, timeout)
	return nil
}

// TestGatewayStartServesV1Cache is the real integration test that replaces the
// former t.Skip stub: it verifies the desktop wiring works by exercising the
// exact call app.startup makes — gateway.Start(ctx, port) — and asserting the
// in-process gateway binds loopback and serves GET /v1/cache. Cancelling the
// context stops the server. No Wails CLI or window is required.
func TestGatewayStartServesV1Cache(t *testing.T) {
	port := freePort(t)
	ctx, cancel := context.WithCancel(context.Background())

	startErr := make(chan error, 1)
	go func() { startErr <- gateway.Start(ctx, port) }()

	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	resp := waitForServer(t, base+"/v1/cache", 3*time.Second)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/cache: expected 200, got %d", resp.StatusCode)
	}
	// Body must be a valid CacheReport (zero-valued here: no trace configured).
	var report map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		t.Fatalf("decode cache report: %v", err)
	}
	for _, field := range []string{
		"total_usage_turns", "cache_hit_tokens", "cache_miss_tokens",
		"output_tokens", "cost_cny", "full_body_evictions",
		"max_miss_tokens", "cache_hit_rate",
	} {
		if _, ok := report[field]; !ok {
			t.Errorf("cache report missing field %q", field)
		}
	}

	// Cancelling the context shuts the server down cleanly.
	cancel()
	select {
	case err := <-startErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("gateway.Start returned error on shutdown: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("gateway.Start did not return after context cancellation")
	}
}

// TestDefaultGatewayPort documents the contract that the desktop wrapper and
// the SPA agree on the port (matches the Vite proxy and App.GetPort()).
func TestDefaultGatewayPort(t *testing.T) {
	if defaultGatewayPort != 7432 {
		t.Fatalf("defaultGatewayPort = %d, want 7432 (Vite proxy + App.GetPort contract)", defaultGatewayPort)
	}
	if newApp(defaultGatewayPort).GetPort() != 7432 {
		t.Fatal("App.GetPort() must report the default gateway port")
	}
}
