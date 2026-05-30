package llm

type DeepSeekProvider struct {
	*Client
}

var _ Provider = (*DeepSeekProvider)(nil)

func newDeepSeekProvider(cfg ProviderConfig) (*DeepSeekProvider, error) {
	c := NewClient(cfg.APIKey, cfg.BaseURL)
	applyProviderTimeouts(c, cfg)
	return &DeepSeekProvider{Client: c}, nil
}

func (DeepSeekProvider) Name() string { return "deepseek" }

func (p DeepSeekProvider) Capabilities() Capabilities {
	return Capabilities{
		Thinking:         true,
		PrefixCache:      true,
		JSONMode:         true,
		MaxContextTokens: 1_000_000,
		MaxOutputTokens:  384_000,
		ReasoningEfforts: []ReasoningEffort{
			ReasoningEffortLow,
			ReasoningEffortMedium,
			ReasoningEffortHigh,
			ReasoningEffortMax,
		},
		ChatPrefixCompletion:   true,
		FIM:                    true,
		FIMRequiresThinkingOff: true,
		SupportsModels: []string{
			"deepseek-v4-flash",
			"deepseek-v4-pro",
			// Legacy aliases — accepted until 2026-07-24 retirement.
			"deepseek-chat",
			"deepseek-reasoner",
		},
	}
}

func (p DeepSeekProvider) BaseClient() *Client { return p.Client }
