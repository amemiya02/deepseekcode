//go:build withwebapp

// Package webapp embeds the compiled Svelte SPA for use by dsc serve --http
// and the Wails desktop wrapper.
//
// This file is only compiled when the withwebapp build tag is set, which
// requires webapp/dist/ to have been populated first (e.g. via `make web`).
// Without the tag, handler_stub.go provides a no-op Handler so that
// `go build ./...` succeeds on a fresh checkout.
//
// To build with the SPA embedded:
//
//	make web && go build -tags withwebapp ./...
package webapp

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed dist
var dist embed.FS

// Handler returns an http.Handler that serves the compiled SPA.
// It must be mounted at "/" AFTER any API routes so API routes take precedence.
//
// TODO(serve-http): wire into cmd/dsc/serve.go once the HTTP server scaffold
// lands — mount Handler() at "/" after registering the /api/* routes.
func Handler() http.Handler {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic("webapp: embed FS: " + err.Error())
	}
	return http.FileServer(http.FS(sub))
}
