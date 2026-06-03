package webapp_test

import (
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/webapp"
)

// TestHandlerReturnsHTML verifies that Handler() serves an HTML response for
// GET /. The test runs under the default (non-withwebapp) build tag, which
// wires in handler_stub.go — no compiled SPA assets are required.
//
// When built with -tags withwebapp the real embed.FS is used; the test still
// passes as long as webapp/dist/index.html contains an HTML document (which
// `make web` guarantees).
func TestHandlerReturnsHTML(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	webapp.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", ct)
	}
}

// TestDistFSNonNil verifies that DistFS() returns a non-nil fs.FS.
// A nil return would panic in the Wails asset server at startup.
func TestDistFSNonNil(t *testing.T) {
	fsys := webapp.DistFS()
	if fsys == nil {
		t.Fatal("DistFS() returned nil")
	}
}

// TestDistFSIndexHTML verifies that DistFS() exposes index.html without error.
// Under the withwebapp build tag the real embed.FS is exercised; under the
// stub tag the stubFS must also serve index.html (per the Wails bootstrap
// requirement — the webview loads index.html first).
func TestDistFSIndexHTML(t *testing.T) {
	fsys := webapp.DistFS()
	f, err := fsys.Open("index.html")
	if err != nil {
		t.Fatalf("DistFS().Open(\"index.html\") error: %v", err)
	}
	_ = f.Close()
}

// TestDistFSUnknownPathReturnsNotExist verifies that DistFS() returns
// fs.ErrNotExist for an unknown path. This guards against a stub that
// silently swallows errors and returns empty content instead of a proper
// not-found result.
func TestDistFSUnknownPathReturnsNotExist(t *testing.T) {
	fsys := webapp.DistFS()
	_, err := fsys.Open("this-path-does-not-exist.xyz")
	if err == nil {
		t.Fatal("expected error for unknown path, got nil")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected fs.ErrNotExist, got: %v", err)
	}
}
