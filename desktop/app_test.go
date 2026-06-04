package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// TestAppImplementsServiceLifecycle verifies App satisfies the v3 service
// interfaces the migration relies on: ServiceName (so bindings/logs use the
// stable "App" name the desktopBridge calls, e.g. "App.GetVersion") and
// ServiceStartup (the v3 replacement for the v2 OnStartup hook). These are
// compile-time + value assertions; a regression in the lifecycle wiring would
// fail to build or report the wrong service name.
func TestAppImplementsServiceLifecycle(t *testing.T) {
	var a any = newApp(0)

	named, ok := a.(application.ServiceName)
	if !ok {
		t.Fatal("App must implement application.ServiceName for stable v3 bindings")
	}
	if got := named.ServiceName(); got != "App" {
		t.Fatalf("ServiceName() = %q, want \"App\" (desktopBridge calls App.GetVersion etc.)", got)
	}

	if _, ok := a.(application.ServiceStartup); !ok {
		t.Fatal("App must implement application.ServiceStartup (v3 OnStartup hook)")
	}
}

// TestServiceStartupLaunchesGateway verifies the v3 ServiceStartup hook starts
// the in-process gateway on the configured port, mirroring the v2 OnStartup
// behaviour. It binds the app context (cancelled by the v3 runtime on exit) and
// the gateway goroutine stops when that context is cancelled.
func TestServiceStartupLaunchesGateway(t *testing.T) {
	port := freePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app := newApp(port)
	if err := app.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup returned error: %v", err)
	}
	if app.ctx != ctx {
		t.Fatal("ServiceStartup must retain the application context for ctx-cancel-on-close")
	}

	// The gateway goroutine should come up on the configured port.
	resp := waitForServer(t, fmt.Sprintf("http://127.0.0.1:%d/v1/cache", port), 3*time.Second)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("gateway not serving after ServiceStartup: status %d", resp.StatusCode)
	}
}

// TestGetPortReturnsConfiguredValue verifies that GetPort returns the port
// value that was passed to newApp. This is a pure function test with no
// Wails or display dependency.
func TestGetPortReturnsConfiguredValue(t *testing.T) {
	const want = 9876
	app := newApp(want)
	if got := app.GetPort(); got != want {
		t.Fatalf("GetPort() = %d, want %d", got, want)
	}
}

// TestGetVersionNonEmpty verifies that GetVersion returns a non-trivial string.
// A broken version wiring (e.g., a missing ldflags injection or wrong import)
// would be caught here before it reaches the Wails JS bridge.
// We require the result to be non-empty after trimming whitespace AND to
// have at least 3 characters, ensuring a placeholder like " " or "v" alone
// does not slip through. (Dev builds legitimately return "dev (none, unknown)"
// which has no digits, so we do not require a digit.)
func TestGetVersionNonEmpty(t *testing.T) {
	app := newApp(0)
	v := app.GetVersion()
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		t.Fatalf("GetVersion() returned empty or whitespace-only string: %q", v)
	}
	// Require at least 3 characters so a single-character or trivially short
	// placeholder (e.g. "v", " ", "0") would be caught.
	if len(trimmed) < 3 {
		t.Fatalf("GetVersion() = %q: version string too short (len=%d), expected a meaningful value", v, len(trimmed))
	}
}
