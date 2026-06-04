// Command dsc-desktop is the Wails v3 desktop wrapper for DeepSeekCode.
//
// It launches a Wails v3 window that loads the compiled React SPA from the
// embedded webapp package. The gateway port is passed to the frontend via
// App.GetPort() so the SPA can construct /v1/* URLs in Wails dev mode (where
// the Vite proxy is not active).
//
// Gateway wiring: App is registered as a v3 Service; its ServiceStartup hook
// launches the in-process internal/gateway HTTP+SSE server via
// `go gateway.Start(a.ctx, a.port)`. It binds 127.0.0.1:<defaultGatewayPort>,
// matching the Vite proxy and App.GetPort(), and shuts down when the v3
// application context is cancelled on application exit. See app.go.
package main

import (
	"log"

	"github.com/amemiya02/deepseekcode/webapp"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const defaultGatewayPort = 7432

func main() {
	app := newApp(defaultGatewayPort)

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
