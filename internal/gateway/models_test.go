package gateway_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/acp"
	"github.com/amemiya02/deepseekcode/internal/gateway"
)

func TestModelsList(t *testing.T) {
	sm := acp.NewSessionManager(stubAgentFactory)
	h := gateway.NewHandler(sm, "")
	ts := httptest.NewServer(h)
	defer ts.Close()

	r, err := http.Get(ts.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Fatalf("models: got %d", r.StatusCode)
	}
	var out struct {
		Models []string `json:"models"`
		Active string   `json:"active"`
		Effort string   `json:"effort"`
	}
	if err := json.NewDecoder(r.Body).Decode(&out); err != nil {
		t.Fatalf("decode models: %v", err)
	}
	if len(out.Models) == 0 {
		t.Fatal("expected a non-empty models list")
	}
}

func TestModelSelectAndEffort(t *testing.T) {
	sm := acp.NewSessionManager(stubAgentFactory)
	h := gateway.NewHandler(sm, "")
	ts := httptest.NewServer(h)
	defer ts.Close()

	// Select a model from the advertised list.
	lr, _ := http.Get(ts.URL + "/v1/models")
	var list struct {
		Models []string `json:"models"`
	}
	json.NewDecoder(lr.Body).Decode(&list)
	lr.Body.Close()
	want := list.Models[len(list.Models)-1]

	pr, _ := http.Post(ts.URL+"/v1/model", "application/json",
		strings.NewReader(`{"model":"`+want+`"}`))
	if pr.StatusCode != http.StatusOK {
		t.Fatalf("POST /v1/model: got %d", pr.StatusCode)
	}
	pr.Body.Close()

	er, _ := http.Post(ts.URL+"/v1/effort", "application/json",
		strings.NewReader(`{"effort":"high"}`))
	if er.StatusCode != http.StatusOK {
		t.Fatalf("POST /v1/effort: got %d", er.StatusCode)
	}
	er.Body.Close()

	// GET reflects the new selection.
	gr, _ := http.Get(ts.URL + "/v1/models")
	var got struct {
		Active string `json:"active"`
		Effort string `json:"effort"`
	}
	json.NewDecoder(gr.Body).Decode(&got)
	gr.Body.Close()
	if got.Active != want {
		t.Errorf("active = %q, want %q", got.Active, want)
	}
	if got.Effort != "high" {
		t.Errorf("effort = %q, want high", got.Effort)
	}
}

func TestBalance(t *testing.T) {
	sm := acp.NewSessionManager(stubAgentFactory)
	h := gateway.NewHandler(sm, "")
	ts := httptest.NewServer(h)
	defer ts.Close()

	r, _ := http.Get(ts.URL + "/v1/balance")
	if r.StatusCode != http.StatusOK {
		t.Fatalf("balance: got %d", r.StatusCode)
	}
	var out struct {
		Currency   string  `json:"currency"`
		Available  float64 `json:"available"`
		Available_ bool    `json:"is_available"`
	}
	if err := json.NewDecoder(r.Body).Decode(&out); err != nil {
		t.Fatalf("decode balance: %v", err)
	}
	// With no real provider balance_url wired, the gateway reports is_available=false
	// rather than erroring, so the SPA shows "balance unavailable".
	if out.Available_ {
		t.Errorf("expected is_available=false without a balance source")
	}
}
