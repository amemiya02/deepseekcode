package gateway_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/acp"
	"github.com/amemiya02/deepseekcode/internal/gateway"
)

func TestRuntimeEndpoint(t *testing.T) {
	sm := acp.NewSessionManager(stubAgentFactory)
	h := gateway.NewHandler(sm, "")
	ts := httptest.NewServer(h)
	defer ts.Close()

	r, err := http.Get(ts.URL + "/v1/runtime")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Fatalf("runtime: got %d", r.StatusCode)
	}
	var out struct {
		Model   string          `json:"model"`
		Version string          `json:"version"`
		Caps    map[string]bool `json:"caps"`
	}
	if err := json.NewDecoder(r.Body).Decode(&out); err != nil {
		t.Fatalf("decode runtime: %v", err)
	}
	// caps is always present (possibly empty under the bare-id fallback).
	if out.Caps == nil {
		t.Fatalf("runtime: caps key missing from response")
	}
}

func TestCapabilitiesEndpoint(t *testing.T) {
	sm := acp.NewSessionManager(stubAgentFactory)
	h := gateway.NewHandler(sm, "")
	ts := httptest.NewServer(h)
	defer ts.Close()

	r, err := http.Get(ts.URL + "/v1/capabilities")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Fatalf("capabilities: got %d", r.StatusCode)
	}
	var out struct {
		Caps map[string]bool `json:"caps"`
	}
	if err := json.NewDecoder(r.Body).Decode(&out); err != nil {
		t.Fatalf("decode capabilities: %v", err)
	}
	if out.Caps == nil {
		t.Fatalf("capabilities: caps key missing from response")
	}
}

func TestRuntimeRejectsNonGET(t *testing.T) {
	sm := acp.NewSessionManager(stubAgentFactory)
	h := gateway.NewHandler(sm, "")
	ts := httptest.NewServer(h)
	defer ts.Close()

	r, err := http.Post(ts.URL+"/v1/runtime", "application/json", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("runtime POST: got %d, want 405", r.StatusCode)
	}
}
