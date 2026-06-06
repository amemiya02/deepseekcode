package gateway_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/config"
	"github.com/amemiya02/deepseekcode/internal/gateway"
)

func TestConfigGet(t *testing.T) {
	cfg := config.Default()
	cfg.Defaults.Model = "deepseek-v4"
	cfg.UI.Accent = "indigo"
	cfg.UI.Density = "comfortable"
	cfg.API.BaseURL = "https://api.deepseek.com"
	gateway.SetConfigSeam(func() (config.Config, error) { return cfg, nil }, nil)
	t.Cleanup(func() { gateway.ResetConfigSeam() })

	ts := newTestServer(t, "")
	resp, err := http.Get(ts.URL + "/v1/config")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var dto gateway.ConfigDTO
	if err := json.NewDecoder(resp.Body).Decode(&dto); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if dto.Accent != "indigo" {
		t.Errorf("Accent = %q, want indigo", dto.Accent)
	}
	if dto.Density != "comfortable" {
		t.Errorf("Density = %q, want comfortable", dto.Density)
	}
	if dto.BaseURL != "https://api.deepseek.com" {
		t.Errorf("BaseURL = %q", dto.BaseURL)
	}
}

func TestConfigPutRoundTrip(t *testing.T) {
	var saved config.Config
	gateway.SetConfigSeam(
		func() (config.Config, error) { return config.Default(), nil },
		func(c config.Config) error { saved = c; return nil },
	)
	t.Cleanup(func() { gateway.ResetConfigSeam() })

	ts := newTestServer(t, "")
	body := `{"accent":"terracotta","density":"compact","model":"deepseek-v4-pro","autoRoute":true}`
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/v1/config", strings.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if saved.UI.Accent != "terracotta" {
		t.Errorf("saved accent = %q, want terracotta", saved.UI.Accent)
	}
	if saved.UI.Density != "compact" {
		t.Errorf("saved density = %q, want compact", saved.UI.Density)
	}
	if saved.Defaults.Model != "deepseek-v4-pro" {
		t.Errorf("saved model = %q", saved.Defaults.Model)
	}
	if !saved.Routing.AutoRoute {
		t.Error("AutoRoute should be true after PUT")
	}
}

func TestConfigDTO_OmitsDeadFields(t *testing.T) {
	b, _ := json.Marshal(gateway.ConfigDTO{})
	s := string(b)
	if strings.Contains(s, "\"theme\"") || strings.Contains(s, "\"transcriptVerbosity\"") {
		t.Fatalf("DTO must not carry theme/transcriptVerbosity: %s", s)
	}
}
