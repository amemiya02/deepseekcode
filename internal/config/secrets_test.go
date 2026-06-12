package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A provider pointed at a loopback proxy needs no API key — the proxy injects
// auth upstream — so ResolveSecret must succeed with an empty key rather than
// failing with ErrNoAPIKey. (Regression: an explicit second provider made the
// synthesized deepseek provider — base_url=127.0.0.1 — unbuildable.)
func TestResolveSecretLoopbackBaseURLIsKeyless(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // empty secrets dir → no file
	for _, base := range []string{
		"http://127.0.0.1:65173",
		"http://localhost:8080/v1",
		"http://[::1]:9000",
	} {
		key, src, err := ResolveSecretWithSource(ProviderConfigTOML{Type: "deepseek", BaseURL: base})
		if err != nil {
			t.Errorf("loopback %q: unexpected error %v", base, err)
		}
		if key != "" {
			t.Errorf("loopback %q: expected empty key, got %q", base, key)
		}
		if src != SecretSourceNone {
			t.Errorf("loopback %q: source = %q, want %q", base, src, SecretSourceNone)
		}
	}
}

// A non-loopback provider with no key must still fail loudly.
func TestResolveSecretNonLoopbackStillRequiresKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, _, err := ResolveSecretWithSource(ProviderConfigTOML{Type: "deepseek", BaseURL: "https://api.deepseek.com"})
	if !errors.Is(err, ErrNoAPIKey) {
		t.Errorf("non-loopback with no key should error ErrNoAPIKey, got %v", err)
	}
}

func TestSecretsPathXDG(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	want := filepath.Join(dir, "deepseekcode", "secrets.toml")
	if got := SecretsPath(); got != want {
		t.Fatalf("SecretsPath() = %q, want %q", got, want)
	}
}

func TestCheckSecretsFilePermissions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := SecretsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`k = "v"`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckSecretsFilePermissions(); err != nil {
		t.Fatalf("0600 CheckSecretsFilePermissions() = %v", err)
	}

	if runtime.GOOS == "windows" {
		return
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckSecretsFilePermissions(); !errors.Is(err, ErrSecretsPermsTooOpen) {
		t.Fatalf("0644 CheckSecretsFilePermissions() = %v, want ErrSecretsPermsTooOpen", err)
	}
}

func TestResolveSecretExplicit(t *testing.T) {
	got, err := ResolveSecret(ProviderConfigTOML{APIKey: "sk-explicit"})
	if err != nil {
		t.Fatalf("ResolveSecret: %v", err)
	}
	if got != "sk-explicit" {
		t.Fatalf("ResolveSecret = %q, want sk-explicit", got)
	}
}

func TestResolveSecretEnv(t *testing.T) {
	t.Setenv("FOO_SECRET", "bar")
	got, err := ResolveSecret(ProviderConfigTOML{EnvVar: "FOO_SECRET"})
	if err != nil {
		t.Fatalf("ResolveSecret: %v", err)
	}
	if got != "bar" {
		t.Fatalf("ResolveSecret = %q, want bar", got)
	}
}

func TestResolveSecretFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := SecretsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`k = "v"`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveSecret(ProviderConfigTOML{EnvVar: "NONE", SecretsFileKey: "k"})
	if err != nil {
		t.Fatalf("ResolveSecret: %v", err)
	}
	if got != "v" {
		t.Fatalf("ResolveSecret = %q, want v", got)
	}
}

func TestResolveSecretMiss(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	_, err := ResolveSecret(ProviderConfigTOML{Type: "deepseek", EnvVar: "NONE"})
	if err == nil {
		t.Fatal("expected missing secret error")
	}
	if !strings.Contains(err.Error(), "env var") || !strings.Contains(err.Error(), SecretsPath()) {
		t.Fatalf("missing secret error = %q", err.Error())
	}
}
