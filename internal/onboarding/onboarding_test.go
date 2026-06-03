package onboarding_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/config"
	"github.com/amemiya02/deepseekcode/internal/onboarding"
)

func TestNeedsOnboarding(t *testing.T) {
	cases := []struct {
		name    string
		cfg     config.Config
		want    bool
		wantErr bool
	}{
		{"empty key, empty base url", config.Config{}, true, false},
		{"key present, url present", config.Config{
			API: config.APIConfig{Key: "sk-test", BaseURL: "https://api.deepseek.com/v1"},
		}, false, false},
		{"key empty, url present", config.Config{
			API: config.APIConfig{BaseURL: "https://api.deepseek.com/v1"},
		}, true, false},
		// Providers-map branch — key present: should NOT need onboarding.
		{"providers map key present", config.Config{
			API: config.APIConfig{BaseURL: "https://api.deepseek.com/v1"},
			Active: config.ActiveConfig{Provider: "deepseek"},
			Providers: map[string]config.ProviderConfigTOML{
				"deepseek": {APIKey: "sk-x"},
			},
		}, false, false},
		// Providers-map branch — empty entry: should need onboarding.
		{"providers map key absent", config.Config{
			API: config.APIConfig{BaseURL: "https://api.deepseek.com/v1"},
			Active: config.ActiveConfig{Provider: "deepseek"},
			Providers: map[string]config.ProviderConfigTOML{
				"deepseek": {},
			},
		}, true, false},
		// I/O error path: secrets file exists with wrong permissions (0644).
		// NeedsOnboarding must propagate the error rather than treating it
		// as an onboarding condition (wantErr: true, want: false).
		{"secrets file perms too open", config.Config{
			API: config.APIConfig{BaseURL: "https://api.deepseek.com/v1"},
			Active: config.ActiveConfig{Provider: "deepseek"},
			Providers: map[string]config.ProviderConfigTOML{
				"deepseek": {},
			},
		}, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Override XDG_CONFIG_HOME so ResolveSecret never touches real
			// secrets files on the developer's machine.
			xdgDir := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", xdgDir)

			// For the permissions-error case, create a secrets file with
			// 0644 permissions so CheckSecretsFilePermissions returns
			// ErrSecretsPermsTooOpen (Unix only; skip on Windows where the
			// check is a no-op).
			if tc.name == "secrets file perms too open" {
				if runtime.GOOS == "windows" {
					t.Skip("permission check is a no-op on Windows")
				}
				secretsDir := filepath.Join(xdgDir, "deepseekcode")
				if err := os.MkdirAll(secretsDir, 0o700); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				secretsFile := filepath.Join(secretsDir, "secrets.toml")
				if err := os.WriteFile(secretsFile, []byte(""), 0o644); err != nil {
					t.Fatalf("write secrets: %v", err)
				}
			}

			got, err := onboarding.NeedsOnboarding(tc.cfg)
			if (err != nil) != tc.wantErr {
				t.Fatalf("NeedsOnboarding() error = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("NeedsOnboarding() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestValidateKey_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Errorf("Authorization header = %q, want %q", got, "Bearer sk-test")
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type header = %q, want %q", got, "application/json")
		}
		// Minimal valid chat-completions response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"chatcmpl-test","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	err := onboarding.ValidateKey(context.Background(), srv.URL, "sk-test", srv.Client())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateKey_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"Invalid API key","type":"invalid_request_error"}}`))
	}))
	defer srv.Close()

	err := onboarding.ValidateKey(context.Background(), srv.URL, "sk-bad", srv.Client())
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
}
