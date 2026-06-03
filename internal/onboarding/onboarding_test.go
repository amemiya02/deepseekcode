package onboarding_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestPersistConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", "") // force ~/.deepseekcode path

	err := onboarding.PersistConfig(onboarding.OnboardingResult{
		APIKey:  "sk-persist-test",
		BaseURL: "https://api.deepseek.com/v1",
		Model:   "deepseek-v4-flash",
	})
	if err != nil {
		t.Fatalf("PersistConfig: %v", err)
	}

	// Secrets file must exist and be 0600
	secretsPath := filepath.Join(dir, ".deepseekcode", "secrets.toml")
	info, err := os.Stat(secretsPath)
	if err != nil {
		t.Fatalf("secrets file not written: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("secrets file perms = %o, want 0600", info.Mode().Perm())
	}

	// Config file must have model and base_url
	cfgPath := filepath.Join(dir, ".deepseek", "config.toml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("config file not written: %v", err)
	}
	if !bytes.Contains(data, []byte("deepseek-v4-flash")) {
		t.Fatalf("config does not contain model: %s", data)
	}
	if !bytes.Contains(data, []byte("api.deepseek.com")) {
		t.Fatalf("config does not contain base_url: %s", data)
	}
}

func TestPersistConfig_OverlayPreservesExistingKeys(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", "") // force ~/.deepseekcode path

	// Pre-write a config.toml with an extra key that PersistConfig should not clobber.
	cfgDir := filepath.Join(dir, ".deepseek")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	existing := []byte("[ui]\ntheme = \"dark\"\n")
	cfgPath := filepath.Join(cfgDir, "config.toml")
	if err := os.WriteFile(cfgPath, existing, 0o600); err != nil {
		t.Fatalf("write existing config: %v", err)
	}

	err := onboarding.PersistConfig(onboarding.OnboardingResult{
		APIKey:  "sk-overlay-test",
		BaseURL: "https://api.deepseek.com/v1",
		Model:   "deepseek-v4-flash",
	})
	if err != nil {
		t.Fatalf("PersistConfig: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("config file not written: %v", err)
	}
	// The extra key from the pre-existing file must survive.
	if !bytes.Contains(data, []byte("dark")) {
		t.Fatalf("overlay destroyed existing ui.theme key; config = %s", data)
	}
	// Our new values must also be present.
	if !bytes.Contains(data, []byte("deepseek-v4-flash")) {
		t.Fatalf("config does not contain model: %s", data)
	}
	if !bytes.Contains(data, []byte("api.deepseek.com")) {
		t.Fatalf("config does not contain base_url: %s", data)
	}
}

func TestRun_NonInteractive_MissingKey(t *testing.T) {
	// Isolate from real secrets files on the developer machine.
	xdgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgDir)
	t.Setenv("DEEPSEEK_API_KEY", "")

	cfg := config.Config{
		API: config.APIConfig{BaseURL: "https://api.deepseek.com/v1"},
	}
	// No key in env, non-interactive → must error with clear message
	var out strings.Builder
	err := onboarding.Run(context.Background(), cfg, []string{"--non-interactive"}, strings.NewReader(""), &out, nil)
	if err == nil {
		t.Fatal("expected error for missing key in non-interactive mode")
	}
	if !strings.Contains(err.Error(), "DEEPSEEK_API_KEY") {
		t.Fatalf("error should mention DEEPSEEK_API_KEY, got: %v", err)
	}
}

func TestRun_NonInteractive_WithKey(t *testing.T) {
	// Provide a fake validate function so no real HTTP.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()

	t.Setenv("DEEPSEEK_API_KEY", "sk-ci-test")
	cfg := config.Config{
		API: config.APIConfig{BaseURL: srv.URL},
	}
	cfg.Providers = map[string]config.ProviderConfigTOML{
		"deepseek": {Type: "deepseek", BaseURL: srv.URL, EnvVar: "DEEPSEEK_API_KEY"},
	}
	cfg.Active.Provider = "deepseek"

	var out strings.Builder
	// Pass srv.Client() as the HTTP client seam via RunOptions.
	err := onboarding.Run(context.Background(), cfg, []string{"--non-interactive"}, strings.NewReader(""), &out, srv.Client())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "success") && !strings.Contains(out.String(), "OK") {
		t.Errorf("expected output to contain 'OK' or 'success', got: %s", out.String())
	}
}

func TestRun_Interactive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"chatcmpl-test","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	// Redirect HOME and XDG so PersistConfig writes to a temp dir.
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", "")

	cfg := config.Config{
		API: config.APIConfig{BaseURL: srv.URL},
	}

	// Simulate user typing: API key, then empty model selection (accept default).
	stdin := strings.NewReader("sk-fake\n\n")
	var out strings.Builder
	err := onboarding.Run(context.Background(), cfg, nil, stdin, &out, srv.Client())
	if err != nil {
		t.Fatalf("runInteractive returned unexpected error: %v", err)
	}

	// Output must confirm success.
	if !strings.Contains(out.String(), "Setup complete") {
		t.Errorf("expected 'Setup complete' in output, got: %s", out.String())
	}

	// PersistConfig must have written the secrets file.
	secretsPath := filepath.Join(dir, ".deepseekcode", "secrets.toml")
	if _, err := os.Stat(secretsPath); err != nil {
		t.Errorf("secrets file not written after interactive run: %v", err)
	}
}
