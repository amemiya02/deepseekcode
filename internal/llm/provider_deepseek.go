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
		SupportsModels:   []string{"deepseek-v4", "deepseek-v4-flash", "deepseek-v4-pro"},
	}
}

func (p DeepSeekProvider) BaseClient() *Client { return p.Client }
