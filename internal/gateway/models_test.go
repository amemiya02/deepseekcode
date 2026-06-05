package gateway_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
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
	// /v1/models now advertises capability descriptor objects (id+label+caps+
	// effort+context), not bare id strings.
	var out struct {
		Models []struct {
			ID string `json:"id"`
		} `json:"models"`
		Active string `json:"active"`
		Effort string `json:"effort"`
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

	// Select a model from the advertised list. Rows are capability descriptors
	// now, so read the id field.
	lr, _ := http.Get(ts.URL + "/v1/models")
	var list struct {
		Models []struct {
			ID string `json:"id"`
		} `json:"models"`
	}
	json.NewDecoder(lr.Body).Decode(&list)
	lr.Body.Close()
	want := list.Models[len(list.Models)-1].ID

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

func TestModelsCapabilityDescriptor(t *testing.T) {
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
		t.Fatalf("status = %d", r.StatusCode)
	}
	var out struct {
		Active string `json:"active"`
		Models []struct {
			ID       string `json:"id"`
			Label    string `json:"label"`
			Provider string `json:"provider"`
			Caps     struct {
				Vision, Tools, Reasoning bool
			} `json:"caps"`
			Effort struct {
				Kind    string   `json:"kind"`
				Levels  []string `json:"levels"`
				Default string   `json:"default"`
			} `json:"effort"`
			Context int `json:"context"`
		} `json:"models"`
	}
	if err := json.NewDecoder(r.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range out.Models {
		if m.ID == "deepseek-v4-flash" {
			found = true
			if m.Context != 1_000_000 {
				t.Errorf("flash context = %d, want 1000000", m.Context)
			}
			if !m.Caps.Reasoning || !m.Caps.Tools {
				t.Errorf("flash caps = %+v, want reasoning+tools", m.Caps)
			}
			if m.Effort.Kind != "levels" {
				t.Errorf("flash effort.kind = %q, want levels", m.Effort.Kind)
			}
			if !slices.Contains(m.Effort.Levels, "max") {
				t.Errorf("flash effort.levels = %v, want to include max", m.Effort.Levels)
			}
			if m.Effort.Default != "max" {
				t.Errorf("flash effort.default = %q, want max", m.Effort.Default)
			}
			if m.Label == "" || m.Label == m.ID {
				t.Errorf("flash label = %q, want a friendly label", m.Label)
			}
		}
		if m.ID == "deepseek-chat" || m.ID == "deepseek-reasoner" {
			t.Errorf("legacy alias %q must NOT be advertised as a picker row", m.ID)
		}
	}
	if !found {
		t.Fatal("deepseek-v4-flash not in descriptor list")
	}
}

func TestEffortAcceptsMax(t *testing.T) {
	sm := acp.NewSessionManager(stubAgentFactory)
	h := gateway.NewHandler(sm, "")
	ts := httptest.NewServer(h)
	defer ts.Close()

	er, err := http.Post(ts.URL+"/v1/effort", "application/json",
		strings.NewReader(`{"effort":"max"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer er.Body.Close()
	if er.StatusCode != http.StatusOK {
		t.Fatalf("POST /v1/effort max = %d, want 200 (max is the configured default)", er.StatusCode)
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
