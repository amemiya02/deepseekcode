package config

import (
	"os"
	"strings"
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

func TestDefaultAutoReasoning(t *testing.T) {
	cfg := Default()
	if cfg.Defaults.AutoReasoning {
		t.Error("AutoReasoning should default false")
	}
}

func TestOverlayAutoReasoning(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.toml"
	body := `
[defaults]
auto_reasoning = true
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Default()
	if err := mergeFile(&cfg, path); err != nil {
		t.Fatal(err)
	}
	if !cfg.Defaults.AutoReasoning {
		t.Error("project auto_reasoning=true should set AutoReasoning to true")
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

func TestDefaultWebConfig(t *testing.T) {
	cfg := Default()
	if !cfg.Web.Enabled {
		t.Error("web.enabled should default true")
	}
	if cfg.Web.SearchProvider != "duckduckgo" {
		t.Errorf("web.search_provider = %q, want duckduckgo", cfg.Web.SearchProvider)
	}
	if cfg.Web.AllowPrivate {
		t.Error("web.allow_private should default false")
	}
}

func TestValidateWebConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     WebConfig
		wantErr bool
		errPath string
	}{
		{
			name:    "valid duckduckgo",
			cfg:     WebConfig{Enabled: true, SearchProvider: "duckduckgo"},
			wantErr: false,
		},
		{
			name:    "valid searxng with base_url",
			cfg:     WebConfig{Enabled: true, SearchProvider: "searxng", SearXNGBaseURL: "https://searx.example.com"},
			wantErr: false,
		},
		{
			name:    "searxng without base_url",
			cfg:     WebConfig{Enabled: true, SearchProvider: "searxng"},
			wantErr: true,
			errPath: "web.searxng_base_url",
		},
		{
			name:    "invalid provider",
			cfg:     WebConfig{Enabled: true, SearchProvider: "google"},
			wantErr: true,
			errPath: "web.search_provider",
		},
		{
			name:    "empty provider is valid (uses default)",
			cfg:     WebConfig{Enabled: true, SearchProvider: ""},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			cfg.Web = tt.cfg
			errs := ValidateStrict(&cfg)
			if tt.wantErr {
				if len(errs) == 0 {
					t.Error("expected validation error, got none")
				} else if tt.errPath != "" && errs[0].Path != tt.errPath {
					t.Errorf("error path = %q, want %q", errs[0].Path, tt.errPath)
				}
			} else {
				for _, e := range errs {
					if strings.HasPrefix(e.Path, "web.") {
						t.Errorf("unexpected web validation error: %s: %s", e.Path, e.Message)
					}
				}
			}
		})
	}
}

func TestWebConfigEnabledFalse(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.toml"
	body := `
[web]
enabled = false
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Default()
	if !cfg.Web.Enabled {
		t.Error("default Web.Enabled should be true")
	}
	if err := mergeFile(&cfg, path); err != nil {
		t.Fatal(err)
	}
	if cfg.Web.Enabled {
		t.Error("web.enabled = false should override the default true")
	}
}
