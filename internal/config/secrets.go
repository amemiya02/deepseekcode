package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"

	"github.com/BurntSushi/toml"
)

var ErrSecretsPermsTooOpen = errors.New("secrets file is not 0600; refusing to load (chmod 600 the file)")

// ErrNoAPIKey is returned by ResolveSecret / ResolveSecretWithSource when no
// API key can be found (no explicit key, the env var is unset, and the secrets
// file does not contain an entry for the provider). It is distinct from I/O
// errors such as bad file permissions or a TOML parse failure.
var ErrNoAPIKey = errors.New("no api key found for provider")

type SecretSource string

const (
	SecretSourceExplicit SecretSource = "explicit"
	SecretSourceEnv      SecretSource = "env"
	SecretSourceFile     SecretSource = "file"
	// SecretSourceNone marks a provider that resolved no key because none is
	// needed — its base_url is a loopback proxy that injects auth upstream.
	SecretSourceNone SecretSource = "none"
)

// loopbackBaseURL reports whether raw points at a loopback host (127.0.0.0/8,
// ::1, or "localhost"). A provider aimed at the local machine is a key-injecting
// proxy/relay the user controls, so dsc may build it without an API key — the
// proxy authenticates upstream. Non-loopback providers still require a key.
func loopbackBaseURL(raw string) bool {
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func ResolveSecret(p ProviderConfigTOML) (apiKey string, err error) {
	key, _, err := ResolveSecretWithSource(p)
	return key, err
}

func ResolveSecretWithSource(p ProviderConfigTOML) (apiKey string, source SecretSource, err error) {
	if p.APIKey != "" {
		return p.APIKey, SecretSourceExplicit, nil
	}
	if p.EnvVar != "" {
		if v := os.Getenv(p.EnvVar); v != "" {
			return v, SecretSourceEnv, nil
		}
	}

	path := SecretsPath()
	if err := CheckSecretsFilePermissions(); err != nil {
		return "", "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if loopbackBaseURL(p.BaseURL) {
				return "", SecretSourceNone, nil
			}
			return "", "", missingSecretError(p, path)
		}
		return "", "", err
	}
	var values map[string]string
	if err := toml.Unmarshal(data, &values); err != nil {
		return "", "", err
	}
	key := p.SecretsFileKey
	if key == "" {
		key = p.EnvVar
	}
	if key != "" {
		if v := values[key]; v != "" {
			return v, SecretSourceFile, nil
		}
	}
	if loopbackBaseURL(p.BaseURL) {
		return "", SecretSourceNone, nil
	}
	return "", "", missingSecretError(p, path)
}

func CheckSecretsFilePermissions() error {
	path := SecretsPath()
	st, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	if st.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%w: chmod 600 %s", ErrSecretsPermsTooOpen, path)
	}
	return nil
}

func SecretsPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "deepseekcode", "secrets.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".deepseekcode", "secrets.toml")
	}
	xdgDefault := filepath.Join(home, ".config", "deepseekcode", "secrets.toml")
	if _, err := os.Stat(xdgDefault); err == nil {
		return xdgDefault
	}
	return filepath.Join(home, ".deepseekcode", "secrets.toml")
}

func missingSecretError(p ProviderConfigTOML, path string) error {
	env := p.EnvVar
	if env == "" {
		env = "<provider env_var>"
	}
	return fmt.Errorf("%w: provider %s (set %s env var or add to %s)", ErrNoAPIKey, p.Type, env, path)
}
