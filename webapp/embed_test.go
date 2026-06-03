package webapp_test

import (
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
