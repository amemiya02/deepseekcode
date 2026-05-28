package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/tools"
)

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

func TestRenderCacheClearResetsStaleEntries(t *testing.T) {
	s := NewScrollback()
	theme := DarkTheme()

	// Add a tool call with tool="old_tool" and render to populate cache.
	s.AppendToolCall("call-1", "old_tool", `{"path":"x.go"}`)
	s.AppendToolResult("call-1", tools.Result{Content: "old output"}, 100*time.Millisecond)
	s.Render(theme, 120)
	oldRender := s.Render(theme, 120)
	if !strings.Contains(oldRender, "old_tool") {
		t.Fatalf("first render should contain old_tool:\n%s", oldRender)
	}

	// Clear — must reset the item render cache.
	s.Clear()

	// Add a new tool call with the SAME callID but different tool name.
	s.AppendToolCall("call-1", "new_tool", `{"path":"x.go"}`)
	s.AppendToolResult("call-1", tools.Result{Content: "new output"}, 200*time.Millisecond)
	newRender := s.Render(theme, 120)

	// The new render must NOT contain the old tool name from the stale cache.
	if strings.Contains(newRender, "old_tool") {
		t.Fatalf("render after Clear should not contain cached old_tool:\n%s", newRender)
	}
	if !strings.Contains(newRender, "new_tool") {
		t.Fatalf("render after Clear should contain new_tool:\n%s", newRender)
	}
}
