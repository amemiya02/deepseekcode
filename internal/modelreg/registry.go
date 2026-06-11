package modelreg

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/amemiya02/deepseekcode/internal/config"
	"github.com/amemiya02/deepseekcode/internal/llm"
)

type Options struct {
	Fetcher Fetcher
	Writer  ConfigWriter
	Builder ProviderBuilder
	TTL     time.Duration
	Now     func() time.Time
}

type Registry struct {
	mu      sync.Mutex
	cfg     config.Config
	sel     Selection
	notice  string
	cache   *fetchCache
	writer  ConfigWriter
	builder ProviderBuilder
}

func New(cfg config.Config, opt Options) *Registry {
	if opt.Fetcher == nil {
		opt.Fetcher = &httpFetcher{}
	}
	if opt.Writer == nil {
		opt.Writer = DefaultWriter{}
	}
	if opt.Builder == nil {
		opt.Builder = DefaultBuilder
	}
	if opt.TTL == 0 {
		opt.TTL = 5 * time.Minute
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
	sel, notice := resolveInitial(cfg)
	return &Registry{
		cfg: cfg, sel: sel, notice: notice,
		cache:  newFetchCache(opt.Fetcher, opt.TTL, opt.Now),
		writer: opt.Writer, builder: opt.Builder,
	}
}

func (r *Registry) Notice() string  { r.mu.Lock(); defer r.mu.Unlock(); return r.notice }
func (r *Registry) Active() Selection { r.mu.Lock(); defer r.mu.Unlock(); return r.sel }

func (r *Registry) providerNames() []string {
	names := make([]string, 0, len(r.cfg.Providers))
	for n := range r.cfg.Providers {
		names = append(names, n)
	}
	sort.Strings(names)
	active := providerOrDefault(r.cfg.Active.Provider)
	out := []string{active}
	for _, n := range names {
		if n != active {
			out = append(out, n)
		}
	}
	return out
}

func (r *Registry) List(ctx context.Context) ([]ModelInfo, error) {
	r.mu.Lock()
	cfg := r.cfg
	names := r.providerNames()
	r.mu.Unlock()

	var out []ModelInfo
	for _, name := range names {
		p := cfg.Providers[name]
		_, secErr := config.ResolveSecret(p)
		available := secErr == nil || name == "deepseek"
		var rows []ModelInfo
		if available {
			rows = hybridCatalog(ctx, r.cache, name, p)
		} else {
			rows = staticCatalog(name, p)
		}
		for i := range rows {
			rows[i].Available = available
			if !available {
				rows[i].Note = "no API key"
			}
		}
		out = append(out, rows...)
	}
	return out, nil
}

func (r *Registry) Refresh(_ context.Context) { r.cache.clear() }

func (r *Registry) Switch(ctx context.Context, provider, model string) (SwitchResult, error) {
	provider = providerOrDefault(provider)
	r.mu.Lock()
	cfg := r.cfg
	r.mu.Unlock()

	p, ok := cfg.Providers[provider]
	if !ok && provider != "deepseek" {
		return SwitchResult{}, fmt.Errorf("provider %q is not configured", provider)
	}
	cat := hybridCatalog(ctx, r.cache, provider, p)
	if !inCatalog(cat, model) && !config.IsLegacyDeepSeekAlias(model) {
		return SwitchResult{}, fmt.Errorf("model %q is not offered by provider %q", model, provider)
	}

	built, err := r.builder(cfg, provider)
	if err != nil {
		return SwitchResult{}, fmt.Errorf("build provider %q: %w", provider, err)
	}

	warning := ""
	if err := r.writer.SetProviderModel(provider, model); err != nil {
		warning = "couldn't save as default: " + err.Error()
	} else if provider != providerOrDefault(cfg.Active.Provider) {
		if err := r.writer.SetActiveProvider(provider); err != nil {
			warning = "couldn't save active provider: " + err.Error()
		}
	}

	levels := effortLevels(built.Caps)
	effort := clampEffort(r.Active().Effort, levels)

	r.mu.Lock()
	r.sel = Selection{Provider: provider, Model: model, Effort: effort}
	r.cfg.Active.Provider = provider
	if r.cfg.Providers != nil {
		pp := r.cfg.Providers[provider]
		pp.DefaultModel = model
		r.cfg.Providers[provider] = pp
	}
	r.mu.Unlock()

	return SwitchResult{
		Selection: Selection{Provider: provider, Model: model, Effort: effort},
		Client: built.Client, Caps: built.Caps, EffortLevels: levels, Warning: warning,
	}, nil
}

func effortLevels(caps llm.Capabilities) []string {
	out := make([]string, 0, len(caps.ReasoningEfforts))
	for _, e := range caps.ReasoningEfforts {
		out = append(out, string(e))
	}
	return out
}

func clampEffort(cur string, levels []string) string {
	if cur == "" || len(levels) == 0 {
		if len(levels) == 0 {
			return ""
		}
		return cur
	}
	for _, l := range levels {
		if l == cur {
			return cur
		}
	}
	return levels[len(levels)-1]
}
