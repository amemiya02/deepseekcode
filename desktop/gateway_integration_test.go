package main

import (
	"testing"
)

// TestGatewayStartIsWired is a compile-time tracking stub that will fail
// until internal/gateway is implemented and wired into main().
//
// When internal/gateway ships a Start(ctx context.Context, port int) function,
// replace this stub with an integration test that:
//  1. Calls gateway.Start with a cancellable context and a random free port.
//  2. Makes a GET /v1/health (or equivalent) request to that port.
//  3. Cancels the context and verifies the server stops accepting connections.
//
// Until then, this test documents the gap: the desktop wrapper opens the
// Wails window but the gateway goroutine is absent, so /v1/* API calls from
// the SPA will get connection-refused. See the TODO in main() and the
// package-level doc comment for the planned wiring.
func TestGatewayStartIsWired(t *testing.T) {
	t.Skip("internal/gateway not yet implemented — remove this Skip and add real assertions once gateway.Start ships")
}
