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
	err    error
}

func (f *fakeFetcher) Fetch(_ context.Context, _ config.ProviderConfigTOML) ([]string, error) {
	atomic.AddInt32(&f.calls, 1)
	return f.models, f.err
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
