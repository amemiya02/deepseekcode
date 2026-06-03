package acp

import (
	"fmt"

	"github.com/amemiya02/deepseekcode/internal/agent"
	"github.com/amemiya02/deepseekcode/internal/config"
	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// RealAgentFactory is an AgentFactory that creates a production Agent per session.
// It reads configuration from the standard config path and constructs all agent
// dependencies. It does NOT alter any DeepSeek wire bytes.
func RealAgentFactory(workingDir string) (AgentRunner, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("acp: load config: %w", err)
	}
	return realAgentFactoryFrom(cfg, workingDir)
}

// realAgentFactoryFrom constructs an AgentRunner from an already-loaded Config.
// It is separated from RealAgentFactory so tests can inject a synthetic config
// without touching the filesystem or requiring real credentials.
func realAgentFactoryFrom(cfg config.Config, workingDir string) (AgentRunner, error) {
	// Resolve provider and build the LLM client.
	provName := cfg.Active.Provider
	if provName == "" {
		provName = "deepseek"
	}
	pcfg, ok := cfg.Providers[provName]
	if !ok {
		return nil, fmt.Errorf("acp: active provider %q is not configured", provName)
	}
	apiKey, err := config.ResolveSecret(pcfg)
	if err != nil {
		return nil, fmt.Errorf("acp: resolve secret: %w", err)
	}
	model := cfg.Defaults.Model
	if !cfg.DefaultsModelExplicit && pcfg.DefaultModel != "" {
		model = pcfg.DefaultModel
	}
	prov, err := llm.NewProvider(pcfg.Type, llm.ProviderConfig{
		Name:                provName,
		BaseURL:             pcfg.BaseURL,
		APIKey:              apiKey,
		FirstTokenTimeoutMs: pcfg.FirstTokenTimeoutMs,
		ChunkStallTimeoutMs: pcfg.ChunkStallTimeoutMs,
		DefaultModel:        model,
	})
	if err != nil {
		return nil, fmt.Errorf("acp: create provider: %w", err)
	}
	client := prov.BaseClient()

	// Build tools registry with builtin tools for the session's working dir.
	reg := tools.New()
	tools.RegisterBuiltins(reg, cfg.Tools.MaxReadBytes, cfg.Tools.MaxWriteBytes, workingDir)

	// Build permissions policy using the default mode.
	pol := permissions.New(
		permissions.ModeDefault,
		workingDir,
		cfg.Permissions.SecretPathPatterns,
		cfg.Permissions.AllowBash,
		nil, // no rule engine in headless mode
	)

	a := agent.New(client, reg, pol, model)

	return NewAgentAdapter(a), nil
}
