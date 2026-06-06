package gateway_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/config"
	"github.com/amemiya02/deepseekcode/internal/gateway"
)

func TestOnboardingStatusNeeded(t *testing.T) {
	cfg := config.Default()
	cfg.API.Key = ""                              // no key resolvable -> needs onboarding
	cfg.Providers = map[string]config.ProviderConfigTOML{} // clear providers so env-var lookup is skipped
	gateway.SetConfigSeam(func() (config.Config, error) { return cfg, nil }, nil)
	t.Cleanup(func() { gateway.ResetConfigSeam() })

	ts := newTestServer(t, "")
	resp, err := http.Get(ts.URL + "/v1/onboarding")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		NeedsOnboarding bool   `json:"needsOnboarding"`
		BaseURL         string `json:"baseUrl"`
		Model           string `json:"model"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !out.NeedsOnboarding {
		t.Error("expected needsOnboarding=true with no key")
	}
	if out.BaseURL == "" {
		t.Error("expected default baseUrl to be suggested")
	}
}

func TestConnectKeyValidateFails(t *testing.T) {
	gateway.SetValidateKeySeam(func(_ context.Context, baseURL, apiKey string, _ *http.Client) error {
		return errBadKey
	})
	t.Cleanup(func() { gateway.ResetValidateKeySeam() })

	ts := newTestServer(t, "")
	body := `{"apiKey":"sk-bad","baseUrl":"https://api.deepseek.com","model":"deepseek-v4"}`
	resp, err := http.Post(ts.URL+"/v1/connect-key", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for invalid key", resp.StatusCode)
	}
}

func TestConnectKeySucceeds(t *testing.T) {
	gateway.SetValidateKeySeam(func(_ context.Context, _, _ string, _ *http.Client) error { return nil })
	var persisted bool
	gateway.SetPersistSeam(func(baseURL, apiKey, model string) error { persisted = true; return nil })
	t.Cleanup(func() { gateway.ResetValidateKeySeam(); gateway.ResetPersistSeam() })

	ts := newTestServer(t, "")
	body := `{"apiKey":"sk-good","baseUrl":"https://api.deepseek.com","model":"deepseek-v4"}`
	resp, err := http.Post(ts.URL+"/v1/connect-key", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if !persisted {
		t.Error("expected PersistConfig to be called on a valid key")
	}
}

var errBadKey = errTest("invalid key")

type errTest string

func (e errTest) Error() string { return string(e) }
