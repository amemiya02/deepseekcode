package modelreg

import (
	"context"
	"sync"
	"time"

	"github.com/amemiya02/deepseekcode/internal/config"
	"github.com/amemiya02/deepseekcode/internal/llm"
)

type cacheEntry struct {
	models []FetchedModel
	expiry time.Time
}

type fetchCache struct {
	mu      sync.Mutex
	entries map[string]cacheEntry
	flight  map[string]*sync.WaitGroup
	f       Fetcher
	ttl     time.Duration
	now     func() time.Time
}

func newFetchCache(f Fetcher, ttl time.Duration, now func() time.Time) *fetchCache {
	return &fetchCache{
		entries: map[string]cacheEntry{},
		flight:  map[string]*sync.WaitGroup{},
		f:       f,
		ttl:     ttl,
		now:     now,
	}
}

func (c *fetchCache) get(ctx context.Context, provider string, p config.ProviderConfigTOML) ([]FetchedModel, error) {
	c.mu.Lock()
	if e, ok := c.entries[provider]; ok && c.now().Before(e.expiry) {
		c.mu.Unlock()
		return e.models, nil
	}
	if wg, ok := c.flight[provider]; ok {
		c.mu.Unlock()
		wg.Wait()
		c.mu.Lock()
		e := c.entries[provider]
		c.mu.Unlock()
		return e.models, nil
	}
	wg := &sync.WaitGroup{}
	wg.Add(1)
	c.flight[provider] = wg
	c.mu.Unlock()

	models, err := c.f.Fetch(ctx, p)

	c.mu.Lock()
	delete(c.flight, provider)
	if err == nil {
		c.entries[provider] = cacheEntry{models: models, expiry: c.now().Add(c.ttl)}
	}
	c.mu.Unlock()
	wg.Done()
	return models, err
}

func (c *fetchCache) clear() {
	c.mu.Lock()
	c.entries = map[string]cacheEntry{}
	c.mu.Unlock()
}

func hybridCatalog(ctx context.Context, c *fetchCache, name string, p config.ProviderConfigTOML) []ModelInfo {
	stat := staticCatalog(name, p)

	// DeepSeek's built-in capabilities are authoritative (known models, known
	// 1M context), so never hit the network for it. Every other provider gets a
	// best-effort /models query so its context window is discovered dynamically
	// rather than assumed from the openai-compat capability default.
	if providerType(name, p) == "deepseek" {
		return stat
	}

	fetched, err := c.get(ctx, name, p)
	ctxByID := map[string]int{}
	if err == nil {
		for _, fm := range fetched {
			if fm.ContextTokens > 0 {
				ctxByID[fm.ID] = fm.ContextTokens
			}
		}
	}

	// Declared / built-in rows are the authoritative set; overlay the fetched
	// context window onto each matching row (fetch wins over the config/cap
	// fallback already baked into stat by staticCatalog).
	if len(stat) > 0 && stat[0].Source != SourceDefault {
		for i := range stat {
			if cw := ctxByID[stat[i].ID]; cw > 0 {
				stat[i].Caps.MaxContextTokens = cw
			}
		}
		return stat
	}

	// No declared/built-in list: the fetched models are the catalog. Context per
	// model is fetch → configured max_context_tokens → capability default.
	if err == nil && len(fetched) > 0 {
		caps, _ := llm.ProviderCapabilities(providerType(name, p))
		out := make([]ModelInfo, 0, len(fetched))
		for _, fm := range fetched {
			rowCaps := caps
			switch {
			case fm.ContextTokens > 0:
				rowCaps.MaxContextTokens = fm.ContextTokens
			case p.MaxContextTokens > 0:
				rowCaps.MaxContextTokens = p.MaxContextTokens
			}
			out = append(out, ModelInfo{
				Provider: name, ID: fm.ID, Label: fm.ID, Caps: rowCaps,
				Source: SourceFetched, Available: true,
			})
		}
		return out
	}
	return stat
}
