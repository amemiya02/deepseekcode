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
	"io/fs"
	"net/http"
	"strings"
	"time"
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

// DistFS returns a stub filesystem for use by Wails's asset server when the
// compiled SPA is not available (build tag withwebapp is absent).
func DistFS() fs.FS {
	return stubFS{}
}

// stubFS is a minimal fs.FS that serves the stub HTML page.
// Open returns a readable file for "index.html" and "." (directory),
// and fs.ErrNotExist for every other path. This ensures the Wails
// asset server can bootstrap the webview without a blank page when the
// withwebapp build tag is absent.
type stubFS struct{}

func (stubFS) Open(name string) (fs.File, error) {
	switch name {
	case "index.html":
		return &stubFile{r: strings.NewReader(stubHTML), name: "index.html", size: int64(len(stubHTML))}, nil
	case ".":
		return &stubDir{}, nil
	default:
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
}

// stubFile implements fs.File for the stub HTML content.
type stubFile struct {
	r    *strings.Reader
	name string
	size int64
}

func (f *stubFile) Read(b []byte) (int, error)  { return f.r.Read(b) }
func (f *stubFile) Close() error                { return nil }
func (f *stubFile) Stat() (fs.FileInfo, error)  { return &stubFileInfo{name: f.name, size: f.size}, nil }

// stubDir implements fs.File for the root directory entry.
type stubDir struct{}

func (d *stubDir) Read([]byte) (int, error)        { return 0, &fs.PathError{Op: "read", Path: ".", Err: fs.ErrInvalid} }
func (d *stubDir) Close() error                    { return nil }
func (d *stubDir) Stat() (fs.FileInfo, error)      { return &stubFileInfo{name: ".", isDir: true}, nil }

// stubFileInfo implements fs.FileInfo for stub entries.
type stubFileInfo struct {
	name  string
	size  int64
	isDir bool
}

func (i *stubFileInfo) Name() string      { return i.name }
func (i *stubFileInfo) Size() int64       { return i.size }
func (i *stubFileInfo) Mode() fs.FileMode {
	if i.isDir {
		return fs.ModeDir | 0o555
	}
	return 0o444
}
func (i *stubFileInfo) ModTime() time.Time { return time.Time{} }
func (i *stubFileInfo) IsDir() bool        { return i.isDir }
func (i *stubFileInfo) Sys() interface{}   { return nil }
