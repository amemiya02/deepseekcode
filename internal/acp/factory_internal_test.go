package acp

// White-box behavioural tests for realAgentFactoryFrom. They live in
// package acp (not acp_test) so they can call the unexported helper
// without filesystem access or real credentials.

import (
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/agent"
	"github.com/amemiya02/deepseekcode/internal/config"
)

// minimalCfg returns a Config that is enough for realAgentFactoryFrom to
// succeed when apiKey is non-empty. Uses a localhost placeholder URL to
// avoid coupling unit tests to a live endpoint string.
func minimalCfg(providerType, apiKey string) config.Config {
	cfg := config.Default()
	cfg.Active.Provider = "testprov"
	cfg.Providers = map[string]config.ProviderConfigTOML{
		"testprov": {
			Type:    providerType,
			BaseURL: "http://localhost",
			APIKey:  apiKey,
		},
	}
	return cfg
}

// TestRealAgentFactorySignature is a compile-time check that RealAgentFactory
// satisfies the AgentFactory type alias. If the signature drifts this file
// will not compile.
func TestRealAgentFactorySignature(t *testing.T) {
	var _ AgentFactory = RealAgentFactory
}

// TestRealAgentFactoryFrom_HappyPath verifies that a well-formed config with a
// valid provider type and an explicit API key returns a non-nil *AgentAdapter
// without error.
func TestRealAgentFactoryFrom_HappyPath(t *testing.T) {
	cfg := minimalCfg("deepseek", "sk-test-key")
	runner, err := realAgentFactoryFrom(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("realAgentFactoryFrom() unexpected error: %v", err)
	}
	if runner == nil {
		t.Fatal("realAgentFactoryFrom() returned nil runner, want non-nil")
	}
	// Behavioral assertion: the returned runner must be an *AgentAdapter
	// wrapping a real *agent.Agent, not a hollow stub.
	if _, ok := runner.(*AgentAdapter); !ok {
		t.Fatalf("realAgentFactoryFrom() returned %T, want *AgentAdapter", runner)
	}
}

// TestRealAgentFactoryFrom_UnknownProvider verifies that requesting a provider
// name not present in cfg.Providers returns a descriptive error.
func TestRealAgentFactoryFrom_UnknownProvider(t *testing.T) {
	cfg := config.Default()
	cfg.Active.Provider = "no-such-provider"
	cfg.Providers = map[string]config.ProviderConfigTOML{} // empty — provider missing

	_, err := realAgentFactoryFrom(cfg, t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing provider, got nil")
	}
	if !strings.Contains(err.Error(), "no-such-provider") {
		t.Errorf("error should mention the missing provider name, got: %v", err)
	}
}

// TestRealAgentFactoryFrom_MissingSecret verifies that a provider with no API
// key and no env-var returns an error that mentions secret/key resolution.
func TestRealAgentFactoryFrom_MissingSecret(t *testing.T) {
	// Provider has no APIKey, no EnvVar pointing to a set variable, and no
	// SecretsFileKey — so ResolveSecret must fail.
	cfg := config.Default()
	cfg.Active.Provider = "emptyprov"
	cfg.Providers = map[string]config.ProviderConfigTOML{
		"emptyprov": {
			Type:    "deepseek",
			BaseURL: "http://localhost",
			// APIKey, EnvVar, SecretsFileKey all zero — secret unresolvable.
		},
	}

	_, err := realAgentFactoryFrom(cfg, t.TempDir())
	if err == nil {
		t.Fatal("expected error when secret cannot be resolved, got nil")
	}
	// The error is wrapped with "acp: resolve secret:" by realAgentFactoryFrom.
	if !strings.Contains(err.Error(), "resolve secret") {
		t.Errorf("error should mention resolve secret, got: %v", err)
	}
}

// TestRealAgentFactoryFrom_ProviderDefaultModel verifies that when the config
// does not explicitly set defaults.model (DefaultsModelExplicit == false) and
// the provider has a DefaultModel, the provider's model wins. We assert that
// the constructed AgentAdapter's underlying agent.Agent uses the provider's
// DefaultModel, not the global defaults model.
func TestRealAgentFactoryFrom_ProviderDefaultModel(t *testing.T) {
	const wantModel = "deepseek-v4-pro"

	cfg := minimalCfg("deepseek", "sk-test-key")
	cfg.DefaultsModelExplicit = false
	cfg.Providers["testprov"] = config.ProviderConfigTOML{
		Type:         "deepseek",
		BaseURL:      "http://localhost",
		APIKey:       "sk-test-key",
		DefaultModel: wantModel,
	}

	runner, err := realAgentFactoryFrom(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("realAgentFactoryFrom() unexpected error: %v", err)
	}

	// Unwrap to *AgentAdapter to inspect the underlying agent model.
	aa, ok := runner.(*AgentAdapter)
	if !ok {
		t.Fatalf("realAgentFactoryFrom() returned %T, want *AgentAdapter", runner)
	}
	ag, ok := aa.a.(*agent.Agent)
	if !ok {
		t.Fatalf("AgentAdapter.a is %T, want *agent.Agent", aa.a)
	}
	if ag.Model != wantModel {
		t.Errorf("agent.Model = %q, want %q (provider DefaultModel should win when DefaultsModelExplicit=false)", ag.Model, wantModel)
	}
}

// TestRealAgentFactoryFrom_ProviderFallback verifies the fallback: when
// cfg.Active.Provider is empty, realAgentFactoryFrom defaults to the "deepseek"
// provider name. Removing or renaming the constant would be detected here.
func TestRealAgentFactoryFrom_ProviderFallback(t *testing.T) {
	cfg := config.Default()
	cfg.Active.Provider = "" // empty — triggers the "deepseek" fallback
	cfg.Providers = map[string]config.ProviderConfigTOML{
		"deepseek": {
			Type:    "deepseek",
			BaseURL: "http://localhost",
			APIKey:  "sk-test-key",
		},
	}

	runner, err := realAgentFactoryFrom(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("realAgentFactoryFrom() with empty provider: unexpected error: %v", err)
	}
	if _, ok := runner.(*AgentAdapter); !ok {
		t.Fatalf("realAgentFactoryFrom() returned %T, want *AgentAdapter", runner)
	}
}
