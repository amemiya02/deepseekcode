package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// StatePath returns the path to the machine-managed state overlay file
// (~/.deepseek/state.toml), or "" if no home directory is available.
func StatePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".deepseek", "state.toml")
}

// State is the machine-managed state overlay. Only [ui].theme is managed
// today; unknown keys are preserved on write so future fields survive.
type State struct {
	UI struct {
		Theme string `toml:"theme"`
	} `toml:"ui"`
}

// LoadState reads the state overlay file. A missing file returns a zero
// State with nil error (no overlay). A malformed file returns the decode
// error — callers should treat this as "no overlay" and continue.
func LoadState() (State, error) {
	path := StatePath()
	if path == "" {
		return State{}, nil
	}
	var st State
	if _, err := toml.DecodeFile(path, &st); err != nil {
		if os.IsNotExist(err) {
			return State{}, nil
		}
		return State{}, err
	}
	return st, nil
}

// SaveThemePreference writes the given theme name to [ui].theme in the
// state overlay file, preserving any other keys already present. Creates
// the file and parent directory if needed. Returns an error if no home
// directory is available or the write fails.
func SaveThemePreference(name string) error {
	path := StatePath()
	if path == "" {
		return fmt.Errorf("cannot save theme preference: no home directory")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating state dir: %w", err)
	}

	// Read existing file into a generic map to preserve unknown keys.
	existing := map[string]any{}
	if _, err := toml.DecodeFile(path, &existing); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading state file: %w", err)
	}

	// Ensure [ui] sub-table exists and set theme.
	ui, _ := existing["ui"].(map[string]any)
	if ui == nil {
		ui = map[string]any{}
	}
	ui["theme"] = name
	existing["ui"] = ui

	// Write back.
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating state file: %w", err)
	}
	defer f.Close()
	if err := toml.NewEncoder(f).Encode(existing); err != nil {
		return fmt.Errorf("encoding state file: %w", err)
	}
	return nil
}
