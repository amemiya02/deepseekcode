// Command dsc-desktop is the Wails v2 desktop wrapper for DeepSeekCode.
//
// It launches a Wails window that loads the compiled Svelte SPA.
// The gateway port is passed to the frontend via App.GetPort() so
// the SPA can construct /v1/* URLs in Wails dev mode (where the
// Vite proxy is not active).
//
// Gateway wiring: the OnStartup hook (App.startup) launches the in-process
// internal/gateway HTTP+SSE server via `go gateway.Start(a.ctx, a.port)`. It
// binds 127.0.0.1:<defaultGatewayPort>, matching the Vite proxy and
// App.GetPort(), and shuts down when a.ctx is cancelled on window close.
package main

import (
	"log"

	"github.com/amemiya02/deepseekcode/webapp"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

const defaultGatewayPort = 7432

func main() {
	app := newApp(defaultGatewayPort)

	// The in-process gateway is launched from app.startup (the Wails OnStartup
	// hook) so it shares the Wails-managed context and shuts down on window
	// close. See app.go and the package doc comment.

	err := wails.Run(&options.App{
		Title:  "DeepSeekCode",
		Width:  1280,
		Height: 800,
		AssetServer: &assetserver.Options{
			// Serve the embedded SPA from the webapp package.
			// In production (wails build), this serves from the embedded FS.
			// In dev mode (wails dev), the Vite dev server takes over.
			Assets: webapp.DistFS(),
		},
		BackgroundColour: &options.RGBA{R: 15, G: 23, B: 42, A: 255},
		OnStartup:        app.startup,
		Bind:             []interface{}{app},
	})
	if err != nil {
		log.Fatal("wails.Run:", err)
	}
}
