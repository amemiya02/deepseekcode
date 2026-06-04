// Command dsc-desktop is the Wails v3 desktop wrapper for DeepSeekCode.
//
// It launches a Wails v3 window that loads the compiled React SPA from the
// embedded webapp package. The gateway port is passed to the frontend via
// App.GetPort() so the SPA can construct /v1/* URLs in Wails dev mode (where
// the Vite proxy is not active).
//
// Gateway wiring (two paths over ONE shared handler, built by App.gatewayHandler):
//   - Webview (primary): the gateway is composed into the Wails asset server via
//     AssetOptions.Middleware (gatewayMiddleware) so /v1/* is served at the
//     webview origin. The packaged .app loads the SPA from the asset origin, not
//     from http://127.0.0.1:<port>, so the SPA's relative /v1 fetches resolve
//     same-origin here — no port URL, no CORS, no SPA change.
//   - Loopback (fallback): ServiceStartup also serves the same handler on
//     127.0.0.1:<defaultGatewayPort> via gateway.ServeHandler, so `open
//     http://127.0.0.1:7432` in a browser works and shares session state. It
//     binds loopback and shuts down when the v3 application context is cancelled
//     on exit. See app.go.
package main

import (
	"log"
	"net/http"
	"strings"

	"github.com/amemiya02/deepseekcode/webapp"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const defaultGatewayPort = 7432

func main() {
	app := newApp(defaultGatewayPort)

	// Build the single gateway handler up front so it can be composed into the
	// Wails asset chain below AND served on the loopback port by ServiceStartup
	// (both reference this same instance — see app.go). Without the gateway the
	// app cannot reach the agent, so a build failure is fatal.
	gw := app.gatewayHandler()
	if gw == nil {
		log.Fatal("desktop: gateway handler unavailable (see prior log)")
	}

	// v3 lifecycle phase 1 — create the application. The in-process gateway is
	// launched from App.ServiceStartup (the v3 service lifecycle hook) so it
	// shares the application-managed context and shuts down on exit. See app.go.
	wailsApp := application.New(application.Options{
		Name:        "DeepSeekCode",
		Description: "DeepSeekCode desktop",
		// Bind App's exported methods to the frontend (GetVersion/GetPort/
		// OpenFileDialog). NewService also wires the ServiceStartup hook.
		Services: []application.Service{
			application.NewService(app),
		},
		Assets: application.AssetOptions{
			// Serve the embedded SPA from the webapp package. In production
			// (wails3 build with -tags withwebapp) this serves the embedded FS;
			// in dev mode (wails3 dev) the Vite dev server takes over.
			Handler: application.AssetFileServerFS(webapp.DistFS()),
			// Route /v1/* to the in-process gateway at the WEBVIEW origin so the
			// SPA's relative /v1 fetches (and the SSE EventSource) resolve
			// same-origin. The packaged .app serves the SPA from the Wails asset
			// origin, not from http://127.0.0.1:<port>, so without this every
			// relative /v1 call 404'd against the asset origin. Composing here is
			// zero-SPA-change and needs no CORS. Everything not under /v1/ falls
			// through to the SPA asset server (and Wails' runtime injection).
			Middleware: gatewayMiddleware(gw),
		},
	})

	// v3 lifecycle phase 2 — create the main window.
	wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "DeepSeekCode",
		Width:            1280,
		Height:           800,
		BackgroundColour: application.NewRGBA(15, 23, 42, 255),
	})

	// v3 lifecycle phase 3 — run. Blocks until the application exits; the
	// application context is cancelled on exit, stopping the gateway goroutine.
	if err := wailsApp.Run(); err != nil {
		log.Fatal("wails app.Run:", err)
	}
}

// gatewayMiddleware returns a Wails asset-server middleware that serves /v1/*
// from the in-process gateway and forwards everything else to the default asset
// chain (the SPA file server plus Wails' runtime injection). Wails calls the
// middleware before its internal middlewares and hands it the default chain as
// next, which is exactly the seam we need for same-origin API routing without
// touching the SPA or adding CORS.
//
// The webview path deliberately does NOT go through the gateway's loopback+token
// auth wrapper: Wails fakes a non-loopback RemoteAddr (TEST-NET 192.0.2.1) for
// webview requests, so that wrapper would 403 them. The handler is otherwise the
// same instance the (auth-wrapped) loopback listener serves.
func gatewayMiddleware(gw http.Handler) application.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if gw != nil && strings.HasPrefix(r.URL.Path, "/v1/") {
				gw.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
