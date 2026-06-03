// Command dsc-desktop is the Wails v2 desktop wrapper for DeepSeekCode.
//
// It launches a Wails window that loads the compiled Svelte SPA.
// The gateway port is passed to the frontend via App.GetPort() so
// the SPA can construct /v1/* URLs in Wails dev mode (where the
// Vite proxy is not active).
//
// Gateway wiring follow-on (tracked): internal/gateway has not yet been
// implemented. Once the Plan 4 HTTP/SSE gateway package ships, wire it
// here by calling:
//
//	go gateway.Start(a.ctx, defaultGatewayPort)
//
// inside the OnStartup hook (a.ctx is set there and is cancelled when the
// Wails window closes, so the goroutine terminates cleanly). Until that
// package exists, /v1/* API calls from the SPA will fail with
// connection-refused; see desktop/gateway_integration_test.go for the
// failing stub that tracks this gap.
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

	// TODO(gateway): start the in-process gateway here once internal/gateway
	// is implemented. See package-level doc comment above for details.
	// Tracked by TestGatewayStartIsWired in gateway_integration_test.go.

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
