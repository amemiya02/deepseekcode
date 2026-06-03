//go:build !withwebapp

// Package webapp embeds the compiled Svelte SPA for use by dsc serve --http
// and the Wails desktop wrapper.
//
// This stub is compiled when the withwebapp build tag is NOT set (e.g. on a
// fresh checkout without running `make web`). It allows `go build ./...` and
// `go test ./...` to succeed without the compiled SPA assets in webapp/dist/.
//
// To build with the real SPA embedded:
//
//	make web && go build -tags withwebapp ./...
package webapp

import (
	"net/http"
	"strings"
)

// stubHTML is a minimal HTML page served in place of the compiled SPA when
// the withwebapp build tag is absent. It satisfies the test assertion that
// GET / returns HTTP 200 with Content-Type text/html.
const stubHTML = `<!doctype html>
<html lang="en">
<head><meta charset="UTF-8"><title>DeepSeekCode (dev build)</title></head>
<body><p>SPA not embedded. Run <code>make web &amp;&amp; go build -tags withwebapp</code>.</p></body>
</html>`

// Handler returns an http.Handler that serves a stub page when the compiled
// SPA is not available (build tag withwebapp is absent).
//
// TODO(serve-http): wire into cmd/dsc/serve.go once the HTTP server scaffold
// lands — mount Handler() at "/" after registering the /api/* routes.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = strings.NewReader(stubHTML).WriteTo(w)
	})
}
