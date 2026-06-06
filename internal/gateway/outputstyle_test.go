package gateway_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/acp"
	"github.com/amemiya02/deepseekcode/internal/gateway"
)

func TestOutputStyleSetAndGet(t *testing.T) {
	sm := acp.NewSessionManager(stubAgentFactory)
	h := gateway.NewHandler(sm, "")
	ts := httptest.NewServer(h)
	defer ts.Close()

	pr, _ := http.Post(ts.URL+"/v1/output-style", "application/json",
		strings.NewReader(`{"session_id":"s","style":"concise"}`))
	if pr.StatusCode != http.StatusOK {
		t.Fatalf("POST /v1/output-style: got %d, want 200", pr.StatusCode)
	}
	pr.Body.Close()

	// An unknown style is rejected.
	br, _ := http.Post(ts.URL+"/v1/output-style", "application/json",
		strings.NewReader(`{"session_id":"s","style":"bogus"}`))
	if br.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown style: got %d, want 400", br.StatusCode)
	}
	br.Body.Close()
}
