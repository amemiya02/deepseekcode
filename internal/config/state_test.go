package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/BurntSushi/toml"
)

// setHome sets both HOME and USERPROFILE so os.UserHomeDir() returns tmp
// on every platform (Windows reads USERPROFILE, not HOME).
func setHome(t *testing.T, tmp string) {
	t.Helper()
	t.Setenv("HOME", tmp)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", tmp)
	}
}

func TestSaveAndLoadThemePreference(t *testing.T) {
	tmp := t.TempDir()
	setHome(t, tmp)

	if err := SaveThemePreference("nebula"); err != nil {
		t.Fatalf("SaveThemePreference: %v", err)
	}
	st, err := LoadState()
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if st.UI.Theme != "nebula" {
		t.Fatalf("expected theme 'nebula', got %q", st.UI.Theme)
	}
	expected := filepath.Join(tmp, ".deepseek", "state.toml")
	if got := StatePath(); got != expected {
		t.Errorf("StatePath() = %q, want %q", got, expected)
	}
}

func TestSaveThemePreservesOtherKeys(t *testing.T) {
	tmp := t.TempDir()
	setHome(t, tmp)

	// Pre-write a state.toml with an extra key.
	dir := filepath.Join(tmp, ".deepseek")
	os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "state.toml")
	os.WriteFile(path, []byte("[ui]\n  theme = \"dark\"\n  extra = \"preserved\"\n"), 0o644)

	if err := SaveThemePreference("aurora"); err != nil {
		t.Fatalf("SaveThemePreference: %v", err)
	}

	raw := map[string]any{}
	toml.DecodeFile(path, &raw)
	ui, _ := raw["ui"].(map[string]any)
	if ui == nil {
		t.Fatal("[ui] table missing after save")
	}
	if ui["theme"] != "aurora" {
		t.Errorf("expected theme 'aurora', got %v", ui["theme"])
	}
	if ui["extra"] != "preserved" {
		t.Errorf("expected extra 'preserved', got %v", ui["extra"])
	}
}

func TestLoadPrefersStateOverConfigTheme(t *testing.T) {
	tmp := t.TempDir()
	setHome(t, tmp)

	cfgDir := filepath.Join(tmp, ".deepseek")
	os.MkdirAll(cfgDir, 0o755)
	os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte("[defaults]\n  theme = \"dark\"\n"), 0o644)
	os.WriteFile(filepath.Join(cfgDir, "state.toml"), []byte("[ui]\n  theme = \"midnight\"\n"), 0o644)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Defaults.Theme != "midnight" {
		t.Errorf("expected theme 'midnight' (state overrides config), got %q", cfg.Defaults.Theme)
	}

	os.Remove(filepath.Join(cfgDir, "state.toml"))
	cfg2, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg2.Defaults.Theme != "dark" {
		t.Errorf("expected theme 'dark' (no state overlay), got %q", cfg2.Defaults.Theme)
	}
}

func TestLoadStateMissingIsZero(t *testing.T) {
	tmp := t.TempDir()
	setHome(t, tmp)

	st, err := LoadState()
	if err != nil {
		t.Fatalf("LoadState on missing file: %v", err)
	}
	if st.UI.Theme != "" {
		t.Errorf("expected empty theme on missing file, got %q", st.UI.Theme)
	}
}
