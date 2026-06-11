package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// ConfigRawIO provides pluggable raw-TOML read/write hooks for testing.
type ConfigRawIO struct {
	Read  func() (map[string]any, error)
	Write func(map[string]any) error
}

var rawIO = ConfigRawIO{Read: readRawUserConfig, Write: writeRawUserConfig}

// SetRawIO replaces the raw I/O hooks and returns a restore function.
func SetRawIO(io ConfigRawIO) (restore func()) {
	prev := rawIO
	rawIO = io
	return func() { rawIO = prev }
}

func rawConfigPath() (string, error) {
	dir := UserConfigDir()
	if dir == "" {
		return "", fmt.Errorf("no user config dir")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

func readRawUserConfig() (map[string]any, error) {
	path, err := rawConfigPath()
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if err := toml.Unmarshal(data, &out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func writeRawUserConfig(m map[string]any) error {
	path, err := rawConfigPath()
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(m)
}

// SetActiveProvider writes the [active].provider key in the raw TOML config.
func SetActiveProvider(name string) error {
	m, err := rawIO.Read()
	if err != nil {
		return err
	}
	active, _ := m["active"].(map[string]any)
	if active == nil {
		active = map[string]any{}
	}
	active["provider"] = name
	m["active"] = active
	return rawIO.Write(m)
}

// SetProviderModel writes providers.<provider>.default_model, preserving
// all other keys in the target provider table and sibling providers.
func SetProviderModel(provider, model string) error {
	m, err := rawIO.Read()
	if err != nil {
		return err
	}
	providers, _ := m["providers"].(map[string]any)
	if providers == nil {
		providers = map[string]any{}
	}
	p, _ := providers[provider].(map[string]any)
	if p == nil {
		p = map[string]any{}
	}
	p["default_model"] = model
	providers[provider] = p
	m["providers"] = providers
	return rawIO.Write(m)
}
