package config

import (
	"os"
	"testing"
)

func TestDefaultValid(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "sk-test")
	cfg := Default()
	cfg.API.Key = "" // pretend file didn't set it
	// Manually apply env fallback the same way Load() does.
	cfg.API.Key = os.Getenv("DEEPSEEK_API_KEY")
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}
	if cfg.Defaults.Model != ModelFlash {
		t.Errorf("default model = %q want %q", cfg.Defaults.Model, ModelFlash)
	}
	if !cfg.Defaults.Thinking {
		t.Error("thinking should default true")
	}
	if !cfg.Duet.Enabled {
		t.Error("duet should default enabled")
	}
}

func TestValidateRequiresAPIKey(t *testing.T) {
	cfg := Default()
	cfg.API.Key = ""
	if err := cfg.Validate(); err == nil {
		t.Error("expected error when API key is empty")
	}
}

func TestExpandEnv(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "sk-xyz")
	got := expandEnv("Bearer ${DEEPSEEK_API_KEY}")
	if got != "Bearer sk-xyz" {
		t.Errorf("expandEnv = %q want %q", got, "Bearer sk-xyz")
	}
	// Unknown var passes through unchanged.
	got = expandEnv("${NEVER_SET_XYZ}")
	if got != "${NEVER_SET_XYZ}" {
		t.Errorf("unknown var = %q want passthrough", got)
	}
}

func TestConfigLoadRules(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.toml"
	body := `
[permissions.rules]
allow = [{ tool = "read_file", args = ".*" }]
deny = [{ tool = "bash", args = "rm\\s+-rf" }]
ask = [{ tool = "write_file", args = ".*secret.*" }]
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Default()
	if err := mergeFile(&cfg, path); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Permissions.Rules.Allow) != 1 {
		t.Fatalf("expected 1 allow rule, got %d", len(cfg.Permissions.Rules.Allow))
	}
	if got := cfg.Permissions.Rules.Allow[0].Tool; got != "read_file" {
		t.Fatalf("allow tool = %q, want read_file", got)
	}
	if got := cfg.Permissions.Rules.Allow[0].Args; got != ".*" {
		t.Fatalf("allow args = %q, want .*", got)
	}
	if len(cfg.Permissions.Rules.Deny) != 1 {
		t.Fatalf("expected 1 deny rule, got %d", len(cfg.Permissions.Rules.Deny))
	}
	if got := cfg.Permissions.Rules.Deny[0].Tool; got != "bash" {
		t.Fatalf("deny tool = %q, want bash", got)
	}
	if len(cfg.Permissions.Rules.Ask) != 1 {
		t.Fatalf("expected 1 ask rule, got %d", len(cfg.Permissions.Rules.Ask))
	}
	if got := cfg.Permissions.Rules.Ask[0].Tool; got != "write_file" {
		t.Fatalf("ask tool = %q, want write_file", got)
	}
}

func TestConfigLoadHooks(t *testing.T) {
	dir := t.TempDir()
	tomlPath := dir + "/config.toml"
	toml := `
[[hooks]]
event = "PreToolUse"
type = "subprocess"
command = "echo allow"
timeout_seconds = 10

[[hooks]]
event = "PostToolUse"
type = "builtin"
name = "duet"
`
	if err := os.WriteFile(tomlPath, []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Default()
	if err := mergeFile(&cfg, tomlPath); err != nil {
		t.Fatal(err)
	}

	if len(cfg.Hooks) != 2 {
		t.Fatalf("expected 2 hooks, got %d", len(cfg.Hooks))
	}
	if cfg.Hooks[0].Event != "PreToolUse" {
		t.Errorf("hook[0].event = %q want PreToolUse", cfg.Hooks[0].Event)
	}
	if cfg.Hooks[0].Type != "subprocess" {
		t.Errorf("hook[0].type = %q want subprocess", cfg.Hooks[0].Type)
	}
	if cfg.Hooks[0].Timeout != 10 {
		t.Errorf("hook[0].timeout = %d want 10", cfg.Hooks[0].Timeout)
	}
	if cfg.Hooks[1].Event != "PostToolUse" {
		t.Errorf("hook[1].event = %q want PostToolUse", cfg.Hooks[1].Event)
	}
	if cfg.Hooks[1].Name != "duet" {
		t.Errorf("hook[1].name = %q want duet", cfg.Hooks[1].Name)
	}
}
