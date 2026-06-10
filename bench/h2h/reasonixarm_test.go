package h2h

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteReasonixConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git", "info"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeReasonixConfig(dir, "http://127.0.0.1:54321"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "reasonix.toml"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := string(b)
	for _, want := range []string{
		`base_url    = "http://127.0.0.1:54321"`,
		`model       = "deepseek-v4-flash"`,
		`api_key_env = "DEEPSEEK_API_KEY"`,
		`default_model = "deepseek-flash"`,
	} {
		if !strings.Contains(cfg, want) {
			t.Errorf("reasonix.toml missing %q:\n%s", want, cfg)
		}
	}
	excl, err := os.ReadFile(filepath.Join(dir, ".git", "info", "exclude"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(excl), "reasonix.toml") {
		t.Errorf("git exclude not updated: %q", excl)
	}
}

func TestWriteReasonixConfigNoGitDir(t *testing.T) {
	// A workspace without .git/info must still get its config
	// (exclude update is fail-soft).
	dir := t.TempDir()
	if err := writeReasonixConfig(dir, "http://127.0.0.1:1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "reasonix.toml")); err != nil {
		t.Fatal(err)
	}
}
