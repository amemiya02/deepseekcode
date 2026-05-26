package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/BurntSushi/toml"
)

var ErrSecretsPermsTooOpen = errors.New("secrets file is not 0600; refusing to load (chmod 600 the file)")

type SecretSource string

const (
	SecretSourceExplicit SecretSource = "explicit"
	SecretSourceEnv      SecretSource = "env"
	SecretSourceFile     SecretSource = "file"
)

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
	return fmt.Errorf("no api key found for provider %s (set %s env var or add to %s)", p.Type, env, path)
}
