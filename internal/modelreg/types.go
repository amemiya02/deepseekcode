package modelreg

import (
	"context"
	"github.com/amemiya02/deepseekcode/internal/config"
	"github.com/amemiya02/deepseekcode/internal/llm"
)

type Selection struct{ Provider, Model, Effort string }

type Source int

const (
	SourceDeclared Source = iota
	SourceBuiltin
	SourceFetched
	SourceDefault
)

func (s Source) String() string {
	switch s {
	case SourceDeclared:
		return "declared"
	case SourceBuiltin:
		return "builtin"
	case SourceFetched:
		return "fetched"
	default:
		return "default"
	}
}

type ModelInfo struct {
	Provider  string
	ID        string
	Label     string
	Caps      llm.Capabilities
	Source    Source
	Available bool
	Note      string
}

type SwitchResult struct {
	Selection
	Client       *llm.Client
	Caps         llm.Capabilities
	EffortLevels []string
	Warning      string
}

// FetchedModel is one model discovered from a provider's /models endpoint,
// optionally carrying the context-window size the endpoint reported (0 when it
// did not). The context lets the picker show a real window instead of the
// provider's hardcoded capability default.
type FetchedModel struct {
	ID            string
	ContextTokens int
}

type Fetcher interface {
	Fetch(ctx context.Context, p config.ProviderConfigTOML) ([]FetchedModel, error)
}

type ConfigWriter interface {
	SetActiveProvider(name string) error
	SetProviderModel(provider, model string) error
}

type ProviderBuilder func(cfg config.Config, providerName string) (BuildResult, error)

type BuildResult struct {
	Client *llm.Client
	Caps   llm.Capabilities
}
