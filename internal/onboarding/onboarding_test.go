package onboarding_test

import (
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
