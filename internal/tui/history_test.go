package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestPromptHistoryPrevNextWalk exercises the readline walk: ↑ steps from newest
// toward oldest, ↓ steps back, and stepping past the newest restores the draft.
func TestPromptHistoryPrevNextWalk(t *testing.T) {
	h := newPromptHistory([]string{"one", "two", "three"}, 500)

	// First Prev saves the live draft and lands on the newest entry.
	if got, ok := h.Prev("draft"); !ok || got != "three" {
		t.Fatalf("Prev #1 = %q,%v, want \"three\",true", got, ok)
	}
	if got, ok := h.Prev(""); !ok || got != "two" {
		t.Fatalf("Prev #2 = %q,%v, want \"two\",true", got, ok)
	}
	if got, ok := h.Prev(""); !ok || got != "one" {
		t.Fatalf("Prev #3 = %q,%v, want \"one\",true", got, ok)
	}
	// At the oldest entry, further Prev is a no-op.
	if got, ok := h.Prev(""); ok {
		t.Fatalf("Prev past oldest = %q,%v, want \"\",false", got, ok)
	}

	// Walk back down toward newer.
	if got, ok := h.Next(); !ok || got != "two" {
		t.Fatalf("Next #1 = %q,%v, want \"two\",true", got, ok)
	}
	if got, ok := h.Next(); !ok || got != "three" {
		t.Fatalf("Next #2 = %q,%v, want \"three\",true", got, ok)
	}
	// Stepping past the newest restores the saved draft and returns to idx==-1.
	if got, ok := h.Next(); !ok || got != "draft" {
		t.Fatalf("Next past newest = %q,%v, want \"draft\",true", got, ok)
	}
	// Not browsing anymore: Next is a no-op.
	if got, ok := h.Next(); ok {
		t.Fatalf("Next while not browsing = %q,%v, want \"\",false", got, ok)
	}
}

// TestPromptHistoryDraftSavedOnFirstPrevOnly verifies the live draft is captured
// only on the first Prev (idx==-1), not on subsequent steps.
func TestPromptHistoryDraftSavedOnFirstPrevOnly(t *testing.T) {
	h := newPromptHistory([]string{"a", "b"}, 500)

	if got, _ := h.Prev("the-draft"); got != "b" {
		t.Fatalf("Prev #1 = %q, want \"b\"", got)
	}
	// A later Prev passes a different live string; it must be ignored.
	if got, _ := h.Prev("IGNORED"); got != "a" {
		t.Fatalf("Prev #2 = %q, want \"a\"", got)
	}
	// Walk back: past newest must restore the *original* draft.
	if got, _ := h.Next(); got != "b" {
		t.Fatalf("Next #1 = %q, want \"b\"", got)
	}
	if got, ok := h.Next(); !ok || got != "the-draft" {
		t.Fatalf("Next past newest = %q,%v, want \"the-draft\",true", got, ok)
	}
}

func TestPromptHistoryPrevOnEmpty(t *testing.T) {
	h := newPromptHistory(nil, 500)
	if got, ok := h.Prev("draft"); ok {
		t.Fatalf("Prev on empty ring = %q,%v, want \"\",false", got, ok)
	}
	if got, ok := h.Next(); ok {
		t.Fatalf("Next on empty ring = %q,%v, want \"\",false", got, ok)
	}
}

// TestPromptHistoryAddSkipsEmptyAndDup checks the storage rules: empty / blank
// input is dropped, consecutive duplicates collapse, and the return value
// reports whether the entry was stored.
func TestPromptHistoryAddSkipsEmptyAndDup(t *testing.T) {
	h := newPromptHistory(nil, 500)

	if h.Add("") {
		t.Fatal("Add(\"\") should not store")
	}
	if h.Add("   ") {
		t.Fatal("Add of all-whitespace should not store (trims to empty)")
	}
	if !h.Add("  hello  ") {
		t.Fatal("Add(\"  hello  \") should store")
	}
	if !h.Add("world") {
		t.Fatal("Add(\"world\") should store")
	}
	// Consecutive duplicate (after trim) is skipped.
	if h.Add("world") {
		t.Fatal("consecutive duplicate should not store")
	}
	// A non-consecutive repeat is allowed.
	if !h.Add("hello") {
		t.Fatal("non-consecutive repeat should store")
	}

	want := []string{"hello", "world", "hello"}
	if got := h.Entries(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Entries = %v, want %v", got, want)
	}
}

// TestPromptHistoryAddResetsBrowsing verifies a submit (Add) returns the ring to
// the live-draft state regardless of where browsing left it.
func TestPromptHistoryAddResetsBrowsing(t *testing.T) {
	h := newPromptHistory([]string{"old"}, 500)
	if _, ok := h.Prev("draft"); !ok {
		t.Fatal("Prev should succeed")
	}
	h.Add("fresh")
	// Browsing must be reset: an immediate Next reports not-browsing.
	if _, ok := h.Next(); ok {
		t.Fatal("Next after Add should report not-browsing")
	}
	// And the next Prev should save the *new* draft and land on the newest.
	if got, ok := h.Prev("newdraft"); !ok || got != "fresh" {
		t.Fatalf("Prev after Add = %q,%v, want \"fresh\",true", got, ok)
	}
}

// TestPromptHistoryResetClearsDraft verifies Reset returns to idx==-1 and clears
// the saved draft so a later Next cannot leak stale text.
func TestPromptHistoryResetClearsDraft(t *testing.T) {
	h := newPromptHistory([]string{"x"}, 500)
	if _, ok := h.Prev("stale"); !ok {
		t.Fatal("Prev should succeed")
	}
	h.Reset()
	if _, ok := h.Next(); ok {
		t.Fatal("Next after Reset should report not-browsing")
	}
}

// TestPromptHistoryCapKeepsNewest verifies both the seed path and the runtime
// Add path keep only the newest max entries.
func TestPromptHistoryCapKeepsNewest(t *testing.T) {
	// Seed cap.
	h := newPromptHistory([]string{"a", "b", "c", "d", "e"}, 3)
	if got, want := h.Entries(), []string{"c", "d", "e"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("seed cap Entries = %v, want %v", got, want)
	}

	// Runtime cap.
	h.Add("f")
	if got, want := h.Entries(), []string{"d", "e", "f"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("runtime cap Entries = %v, want %v", got, want)
	}
}

// TestNewPromptHistorySeedDedup verifies the seed path applies the empty and
// consecutive-dup rules.
func TestNewPromptHistorySeedDedup(t *testing.T) {
	h := newPromptHistory([]string{"a", "a", "", "b", "b", "a"}, 500)
	want := []string{"a", "b", "a"}
	if got := h.Entries(); !reflect.DeepEqual(got, want) {
		t.Fatalf("seed dedup Entries = %v, want %v", got, want)
	}
}

// TestPromptHistoryFileRoundTrip writes entries (including a multi-line one) via
// appendPromptHistory and reads them back via loadPromptHistory, asserting the
// newline-bearing entry survives the escape/unescape round-trip exactly.
func TestPromptHistoryFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	// Nested path: appendPromptHistory must mkdir -p the parent.
	path := filepath.Join(dir, "history", "proj.txt")

	lines := []string{
		"first prompt",
		"multi\nline\nprompt",
		"third prompt",
	}
	for _, l := range lines {
		if err := appendPromptHistory(path, l); err != nil {
			t.Fatalf("appendPromptHistory(%q) error: %v", l, err)
		}
	}

	got := loadPromptHistory(path, 500)
	if !reflect.DeepEqual(got, lines) {
		t.Fatalf("round-trip = %#v, want %#v", got, lines)
	}

	// Spot-check that the multi-line entry kept its newlines.
	if got[1] != "multi\nline\nprompt" {
		t.Fatalf("multi-line entry = %q, want \"multi\\nline\\nprompt\"", got[1])
	}

	// And the on-disk form has no embedded newline for that entry (one line each).
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if n := countNewlines(string(raw)); n != len(lines) {
		t.Fatalf("file has %d newlines, want %d (one per entry)", n, len(lines))
	}
}

// TestLoadPromptHistoryMissingFile verifies a missing file returns nil.
func TestLoadPromptHistoryMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.txt")
	if got := loadPromptHistory(path, 500); got != nil {
		t.Fatalf("loadPromptHistory on missing file = %#v, want nil", got)
	}
}

// TestLoadPromptHistoryCapKeepsNewest verifies the loader caps to the newest max
// (file is oldest-first, newest-last).
func TestLoadPromptHistoryCapKeepsNewest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "h.txt")
	for _, l := range []string{"a", "b", "c", "d"} {
		if err := appendPromptHistory(path, l); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	got := loadPromptHistory(path, 2)
	if want := []string{"c", "d"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("capped load = %v, want %v", got, want)
	}
}

func countNewlines(s string) int {
	n := 0
	for _, r := range s {
		if r == '\n' {
			n++
		}
	}
	return n
}
