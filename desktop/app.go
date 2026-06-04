package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/amemiya02/deepseekcode/internal/gateway"
	"github.com/amemiya02/deepseekcode/internal/version"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// App is the Wails v3 application service.
//
// Its exported methods are bound to the frontend via Wails v3's generated
// JS bindings (the SPA reaches them through web/src/lib/desktopBridge.ts).
// App is registered as a v3 Service in main.go; the v3 runtime calls
// ServiceStartup once the application context is available, which is where the
// in-process gateway goroutine is launched.
type App struct {
	ctx  context.Context
	port int

	// gw is the single gateway handler shared by the Wails asset middleware
	// (which serves /v1/* same-origin from the webview, see main.go) and the
	// loopback HTTP listener started in ServiceStartup (the browser fallback).
	// Built once via gatewayHandler so both paths drive the same SessionManager,
	// store and snapshots — sessions created in the window are visible to the
	// browser fallback and vice versa.
	gwOnce sync.Once
	gw     http.Handler
}

func newApp(port int) *App { return &App{port: port} }

// gatewayHandler returns the process-wide gateway handler, building it once on
// first use. main.go calls it to compose /v1/* into the Wails asset chain;
// ServiceStartup calls it to serve the same handler on the loopback port. A nil
// return means DefaultHandler failed (e.g. the session store could not open);
// callers must handle that — the app is unusable without the gateway.
func (a *App) gatewayHandler() http.Handler {
	a.gwOnce.Do(func() {
		h, err := gateway.DefaultHandler()
		if err != nil {
			log.Printf("desktop: build gateway handler: %v", err)
			return
		}
		a.gw = h
	})
	return a.gw
}

// ServiceName satisfies the optional v3 application.ServiceName interface so the
// generated bindings and logs use the stable name "App" (matching the v2
// `Bind: []interface{}{app}` behaviour the SPA bridge expects).
func (a *App) ServiceName() string { return "App" }

// ServiceStartup is the v3 service lifecycle hook (replaces the v2 OnStartup
// callback). It receives the application context, which the v3 runtime cancels
// on shutdown — so the gateway goroutine terminates cleanly with the app, the
// same ctx-cancel-on-close semantics as the v2 wrapper.
//
// Launch the in-process gateway so the SPA's /v1/* calls reach the agent. It
// binds 127.0.0.1:<port> (matching the Vite proxy and App.GetPort()).
func (a *App) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	a.ctx = ctx
	h := a.gatewayHandler()
	if h == nil {
		return fmt.Errorf("desktop: gateway handler unavailable")
	}
	// Serve the SAME handler the asset middleware uses on the loopback port, so
	// the browser fallback (open http://127.0.0.1:<port>) shares session state
	// with the in-window webview. The webview itself reaches /v1/* same-origin
	// through the asset middleware in main.go and does not depend on this listener.
	go func() {
		if err := gateway.ServeHandler(a.ctx, a.port, h); err != nil {
			log.Printf("desktop: gateway stopped: %v", err)
		}
	}()
	return nil
}

// GetVersion returns the dsc binary version string for display in the UI.
func (a *App) GetVersion() string {
	return version.String()
}

// GetPort returns the gateway port so the frontend can construct API URLs
// when running outside the embedded serve path (e.g., Wails dev mode).
func (a *App) GetPort() int {
	return a.port
}

// OpenFileDialog opens the native OS file picker and returns the selected path.
// The frontend calls this through web/src/lib/desktopBridge.ts.
//
// OpenFileDialog cannot be unit-tested without a live Wails context; tested
// manually via wails3 dev.
func (a *App) OpenFileDialog() (string, error) {
	path, err := application.Get().Dialog.OpenFile().
		SetTitle("Open file").
		CanChooseFiles(true).
		AddFilter("All Files", "*.*").
		PromptForSingleSelection()
	if err != nil {
		return "", fmt.Errorf("file dialog: %w", err)
	}
	return path, nil
}
