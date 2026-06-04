package gateway_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/gateway"
)

func TestUpdateAvailable(t *testing.T) {
	gateway.SetUpdateSeam(
		func(_ context.Context) (string, string, error) {
			return "v9.9.9", "https://example.com/releases/v9.9.9", nil
		},
		func() string { return "v1.0.0" },
	)
	t.Cleanup(func() { gateway.ResetUpdateSeam() })

	ts := newTestServer(t, "")
	resp, err := http.Get(ts.URL + "/v1/update")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Current         string `json:"current"`
		Latest          string `json:"latest"`
		UpdateAvailable bool   `json:"updateAvailable"`
		URL             string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !out.UpdateAvailable {
		t.Error("expected updateAvailable=true (v1.0.0 < v9.9.9)")
	}
	if out.URL == "" {
		t.Error("expected a download URL")
	}
}

func TestUpdateUpToDate(t *testing.T) {
	gateway.SetUpdateSeam(
		func(_ context.Context) (string, string, error) { return "v1.0.0", "https://example.com", nil },
		func() string { return "v1.0.0" },
	)
	t.Cleanup(func() { gateway.ResetUpdateSeam() })

	ts := newTestServer(t, "")
	resp, _ := http.Get(ts.URL + "/v1/update")
	defer resp.Body.Close()
	var out struct {
		UpdateAvailable bool `json:"updateAvailable"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.UpdateAvailable {
		t.Error("expected updateAvailable=false when equal")
	}
}
