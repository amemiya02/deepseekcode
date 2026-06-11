package modelreg

import (
	"context"
	"sync"
	"time"

	"github.com/amemiya02/deepseekcode/internal/config"
	"github.com/amemiya02/deepseekcode/internal/llm"
)

type cacheEntry struct {
	models []string
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

func (c *fetchCache) get(ctx context.Context, provider string, p config.ProviderConfigTOML) ([]string, error) {
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
	if len(stat) > 0 && stat[0].Source != SourceDefault {
		return stat
	}
	ids, err := c.get(ctx, name, p)
	if err == nil && len(ids) > 0 {
		caps, _ := llm.ProviderCapabilities(providerType(name, p))
		out := make([]ModelInfo, 0, len(ids))
		for _, id := range ids {
			out = append(out, ModelInfo{
				Provider: name, ID: id, Label: id, Caps: caps,
				Source: SourceFetched, Available: true,
			})
		}
		return out
	}
	return stat
}
