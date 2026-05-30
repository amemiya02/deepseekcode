package llm

import (
	"context"
	"fmt"
	"time"
)

type Capabilities struct {
	Thinking               bool
	PrefixCache            bool
	JSONMode               bool
	MaxContextTokens       int
	MaxOutputTokens        int
	ReasoningEfforts       []ReasoningEffort
	ChatPrefixCompletion   bool
	FIM                    bool
	FIMRequiresThinkingOff bool
	SupportsModels         []string
}

type Provider interface {
	Name() string
	Stream(ctx context.Context, req Request) (<-chan Event, error)
	ValidatePro(ctx context.Context, prompt string) (approve bool, reasoning string, err error)
	Capabilities() Capabilities
	BaseClient() *Client
}

type ProviderConfig struct {
	Name                string
	BaseURL             string
	APIKey              string
	FirstTokenTimeoutMs int
	ChunkStallTimeoutMs int
	DefaultModel        string
	ValidationModel     string
}

func NewProvider(name string, cfg ProviderConfig) (Provider, error) {
	switch name {
	case "", "deepseek":
		return newDeepSeekProvider(cfg)
	case "openai-compat":
		return newOpenAICompatProvider(cfg)
	default:
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
}

func applyProviderTimeouts(c *Client, cfg ProviderConfig) {
	if cfg.FirstTokenTimeoutMs > 0 {
		c.FirstTokenTimeout = time.Duration(cfg.FirstTokenTimeoutMs) * time.Millisecond
	}
	if cfg.ChunkStallTimeoutMs > 0 {
		c.ChunkStallTimeout = time.Duration(cfg.ChunkStallTimeoutMs) * time.Millisecond
	}
}

func (c Capabilities) clone() Capabilities {
	c.SupportsModels = append([]string(nil), c.SupportsModels...)
	c.ReasoningEfforts = append([]ReasoningEffort(nil), c.ReasoningEfforts...)
	return c
}
