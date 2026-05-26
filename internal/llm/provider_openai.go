package llm

import (
	"context"
	"fmt"
)

type OpenAICompatProvider struct {
	client   *Client
	valModel string
}

var _ Provider = (*OpenAICompatProvider)(nil)

func newOpenAICompatProvider(cfg ProviderConfig) (*OpenAICompatProvider, error) {
	c := NewClient(cfg.APIKey, cfg.BaseURL)
	applyProviderTimeouts(c, cfg)
	valModel := cfg.ValidationModel
	if valModel == "" {
		valModel = cfg.DefaultModel
	}
	return &OpenAICompatProvider{client: c, valModel: valModel}, nil
}

func (OpenAICompatProvider) Name() string { return "openai-compat" }

func (OpenAICompatProvider) Capabilities() Capabilities {
	return Capabilities{
		Thinking:         false,
		PrefixCache:      false,
		JSONMode:         true,
		MaxContextTokens: 128_000,
	}
}

func (p OpenAICompatProvider) BaseClient() *Client { return p.client }

func (p OpenAICompatProvider) Stream(ctx context.Context, req Request) (<-chan Event, error) {
	clone := req
	clone.Thinking = nil
	return p.client.Stream(ctx, clone)
}

func (p OpenAICompatProvider) ValidatePro(ctx context.Context, prompt string) (bool, string, error) {
	if p.valModel == "" {
		return false, "", fmt.Errorf("validate model not configured")
	}
	return validateWithModel(ctx, p.client, p.valModel, prompt)
}
