package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// labelHandler writes a fixed label so a test can tell which branch of the
// middleware served the request.
func labelHandler(label string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, label)
	})
}

// serve runs req through h and returns the recorded body.
func serve(h http.Handler, method, target string) string {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec.Body.String()
}

// TestGatewayMiddlewareRoutesV1ToGateway is the runtime-contract guard for the
// .app gateway-404 fix: every /v1/* request (including the SSE EventSource path)
// must reach the gateway handler, while everything else falls through to the SPA
// asset chain (next). A vitest suite cannot catch this because it mocks fetch;
// this exercises the real Go routing the packaged webview depends on.
func TestGatewayMiddlewareRoutesV1ToGateway(t *testing.T) {
	gw := labelHandler("GATEWAY")
	next := labelHandler("SPA")
	h := gatewayMiddleware(gw)(next)

	toGateway := []string{
		"/v1/prompt",
		"/v1/cache",
		"/v1/events?session_id=abc", // SSE stream
		"/v1/sessions",
		"/v1/files?path=.",
		"/v1/mcp",
	}
	for _, target := range toGateway {
		if got := serve(h, http.MethodGet, target); got != "GATEWAY" {
			t.Errorf("%s served by %q, want GATEWAY (must reach the in-process gateway)", target, got)
		}
	}

	toSPA := []string{
		"/",
		"/index.html",
		"/assets/app-abcdef.js",
		"/favicon.svg",
		"/v1",        // exact /v1 (no trailing slash) is not an API route
		"/v1foo/bar", // a path that merely starts with v1 but not /v1/
		"/wails/runtime.js",
	}
	for _, target := range toSPA {
		if got := serve(h, http.MethodGet, target); got != "SPA" {
			t.Errorf("%s served by %q, want SPA (must fall through to the asset chain)", target, got)
		}
	}
}

// TestGatewayMiddlewarePreservesMethod verifies the gateway branch is method
// agnostic — the SPA POSTs to /v1/prompt, so a POST under /v1/ must route to the
// gateway, not fall through.
func TestGatewayMiddlewarePreservesMethod(t *testing.T) {
	h := gatewayMiddleware(labelHandler("GATEWAY"))(labelHandler("SPA"))
	if got := serve(h, http.MethodPost, "/v1/prompt"); got != "GATEWAY" {
		t.Errorf("POST /v1/prompt served by %q, want GATEWAY", got)
	}
}

// TestGatewayMiddlewareNilGatewayFallsThrough verifies the defensive nil guard:
// if the gateway handler could not be built, the middleware must not panic — it
// forwards to next so the SPA still loads (and /v1 calls 404 honestly rather than
// crashing the webview request worker).
func TestGatewayMiddlewareNilGatewayFallsThrough(t *testing.T) {
	h := gatewayMiddleware(nil)(labelHandler("SPA"))
	if got := serve(h, http.MethodGet, "/v1/cache"); got != "SPA" {
		t.Errorf("nil gateway: /v1/cache served by %q, want SPA (fall through, no panic)", got)
	}
}
