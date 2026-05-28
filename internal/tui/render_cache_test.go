package tui

import "testing"

func TestRenderCacheStoresByStableKey(t *testing.T) {
	c := NewRenderCache(2)
	c.Put("a", "rendered-a")
	c.Put("b", "rendered-b")
	if got, ok := c.Get("a"); !ok || got != "rendered-a" {
		t.Fatalf("Get(a) = %q,%v", got, ok)
	}
	c.Put("c", "rendered-c")
	if _, ok := c.Get("b"); ok {
		t.Fatal("expected b to be evicted")
	}
}
