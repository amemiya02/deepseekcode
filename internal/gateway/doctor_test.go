package gateway_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/config"
	"github.com/amemiya02/deepseekcode/internal/gateway"
)

func TestDoctorReturnsChecks(t *testing.T) {
	cfg := config.Default()
	cfg.API.BaseURL = "" // forces at least one failing check deterministically
	gateway.SetConfigSeam(func() (config.Config, error) { return cfg, nil }, nil)
	t.Cleanup(func() { gateway.ResetConfigSeam() })

	ts := newTestServer(t, "")
	resp, err := http.Get(ts.URL + "/v1/doctor")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var out struct {
		AllOK  bool `json:"allOk"`
		Checks []struct {
			Name   string `json:"name"`
			OK     bool   `json:"ok"`
			Detail string `json:"detail"`
		} `json:"checks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Checks) == 0 {
		t.Fatal("expected at least one doctor check")
	}
	for _, c := range out.Checks {
		if c.Name == "" {
			t.Error("a check has an empty name")
		}
	}
}
