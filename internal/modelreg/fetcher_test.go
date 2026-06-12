package modelreg

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/config"
)

type fakeFetcher struct {
	calls  int32
	models []string
	ctx    map[string]int // optional per-id context window
	err    error
}

func (f *fakeFetcher) Fetch(_ context.Context, _ config.ProviderConfigTOML) ([]FetchedModel, error) {
	atomic.AddInt32(&f.calls, 1)
	out := make([]FetchedModel, len(f.models))
	for i, id := range f.models {
		out[i] = FetchedModel{ID: id, ContextTokens: f.ctx[id]}
	}
	return out, f.err
}

func TestCacheTTLAndRefresh(t *testing.T) {
	now := time.Unix(0, 0)
	ff := &fakeFetcher{models: []string{"a", "b"}}
	c := newFetchCache(ff, 5*time.Minute, func() time.Time { return now })
	p := config.ProviderConfigTOML{Type: "openai-compat", BaseURL: "x"}

	if got, _ := c.get(context.Background(), "mimo", p); len(got) != 2 {
		t.Fatalf("first get = %v", got)
	}
	c.get(context.Background(), "mimo", p)
	if n := atomic.LoadInt32(&ff.calls); n != 1 {
		t.Fatalf("calls = %d, want 1 (cached)", n)
	}
	now = now.Add(6 * time.Minute)
	c.get(context.Background(), "mimo", p)
	if n := atomic.LoadInt32(&ff.calls); n != 2 {
		t.Fatalf("calls = %d, want 2 (expired)", n)
	}
	c.clear()
	c.get(context.Background(), "mimo", p)
	if n := atomic.LoadInt32(&ff.calls); n != 3 {
		t.Fatalf("calls = %d, want 3 (cleared)", n)
	}
}

func TestHybridCatalogInsertsFetchedLayer(t *testing.T) {
	now := time.Unix(0, 0)
	ff := &fakeFetcher{models: []string{"mimo-pro", "mimo-flash"}}
	c := newFetchCache(ff, time.Minute, func() time.Time { return now })
	p := config.ProviderConfigTOML{Type: "openai-compat", BaseURL: "x", DefaultModel: "mimo-pro"}
	got := hybridCatalog(context.Background(), c, "mimo", p)
	if len(got) != 2 || got[0].Source != SourceFetched {
		t.Fatalf("hybrid = %+v, want 2 fetched rows", got)
	}
}

func TestHybridCatalogFallsBackToDefaultOnFetchError(t *testing.T) {
	now := time.Unix(0, 0)
	ff := &fakeFetcher{err: context.DeadlineExceeded}
	c := newFetchCache(ff, time.Minute, func() time.Time { return now })
	p := config.ProviderConfigTOML{Type: "openai-compat", BaseURL: "x", DefaultModel: "mimo-pro"}
	got := hybridCatalog(context.Background(), c, "mimo", p)
	if len(got) != 1 || got[0].ID != "mimo-pro" || got[0].Source != SourceDefault {
		t.Fatalf("fallback = %+v, want [mimo-pro/default]", got)
	}
}

// A provider's declared context window (config) overrides the built-in cap so
// the picker shows e.g. 1M instead of the openai-compat 128k default.
func TestStaticCatalogAppliesConfiguredContext(t *testing.T) {
	p := config.ProviderConfigTOML{
		Type: "openai-compat", BaseURL: "x",
		Models: []string{"m1"}, MaxContextTokens: 1_000_000,
	}
	got := staticCatalog("mimo", p)
	if len(got) != 1 || got[0].Caps.MaxContextTokens != 1_000_000 {
		t.Fatalf("configured context not applied: %+v", got)
	}
}

// A live /models context window overlays declared rows (fetch wins over the
// config/cap fallback) so context is discovered dynamically.
func TestHybridCatalogEnrichesDeclaredContextFromFetch(t *testing.T) {
	now := time.Unix(0, 0)
	ff := &fakeFetcher{models: []string{"m1", "m2"}, ctx: map[string]int{"m1": 1_000_000}}
	c := newFetchCache(ff, time.Minute, func() time.Time { return now })
	p := config.ProviderConfigTOML{Type: "openai-compat", BaseURL: "x", Models: []string{"m1", "m2"}}
	got := hybridCatalog(context.Background(), c, "mimo", p)
	byID := map[string]int{}
	for _, m := range got {
		byID[m.ID] = m.Caps.MaxContextTokens
	}
	if byID["m1"] != 1_000_000 {
		t.Fatalf("fetched context not overlaid on declared row m1: %+v", got)
	}
}

// DeepSeek's built-in catalog is authoritative — hybridCatalog must not hit the
// network for it (no fetch call).
func TestHybridCatalogSkipsFetchForDeepSeek(t *testing.T) {
	now := time.Unix(0, 0)
	ff := &fakeFetcher{models: []string{"x"}}
	c := newFetchCache(ff, time.Minute, func() time.Time { return now })
	p := config.ProviderConfigTOML{} // deepseek built-in
	_ = hybridCatalog(context.Background(), c, "deepseek", p)
	if n := atomic.LoadInt32(&ff.calls); n != 0 {
		t.Fatalf("deepseek should not fetch, got %d calls", n)
	}
}
