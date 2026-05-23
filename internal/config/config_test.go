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
