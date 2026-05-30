package tui

import (
	"strings"
	"testing"
)

// TestScrollbackClearInvalidatesRenderCache pins the root cause of the
// "/clear does nothing" bug: Render() caches a "finished prefix" keyed on
// structureSeq, and Clear() must bump structureSeq so the next Render()
// rebuilds from the (now empty) item list instead of replaying the stale
// pre-clear prefix. This is a unit test on Scrollback directly.
func TestScrollbackClearInvalidatesRenderCache(t *testing.T) {
	sb := NewScrollback()
	th := DarkTheme()

	sb.AppendUser("hello world")
	full := sb.Render(th, 80) // populates finishedPrefix + line caches
	if !strings.Contains(full, "hello world") {
		t.Fatalf("precondition: render should contain appended text; got %q", full)
	}

	sb.Clear()
	cleared := sb.Render(th, 80)
	if strings.Contains(cleared, "hello world") {
		t.Fatalf("Clear did not invalidate the render cache; Render still replays stale content: %q", cleared)
	}
	if n := len(sb.Items()); n != 0 {
		t.Fatalf("Clear left %d items", n)
	}
}

// TestClearTypedRepaintsView pins the user-facing symptom through the real
// key path: type "/clear" and press Enter. The existing /clear tests only
// assert the item slice empties; they never re-render, so they missed that
// the viewport kept showing the pre-clear content.
func TestClearTypedRepaintsView(t *testing.T) {
	a := sizeApp(t, newKeyflowApp(t), 100, 40)
	a.scrollback.AppendUser("hello world")
	a.refreshView()
	if !strings.Contains(a.View().Content, "hello world") {
		t.Fatal("precondition: view should show the appended text before /clear")
	}

	for _, r := range "/clear" {
		a = drive(t, a, press(r))
	}
	a = drive(t, a, keyEnter())

	if n := len(a.scrollback.Items()); n != 0 {
		t.Fatalf("/clear left %d items", n)
	}
	if strings.Contains(a.View().Content, "hello world") {
		t.Fatal("after /clear the view STILL shows 'hello world' — no repaint")
	}
}
