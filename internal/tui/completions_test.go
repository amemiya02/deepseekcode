package tui

import (
	"strings"
	"testing"
)

// sampleItems builds a small candidate set spanning the three cmd kinds so the
// completion tests exercise filtering, ordering, and rendering uniformly.
func sampleItems() []complItem {
	return []complItem{
		{insert: "/models", label: "/models", detail: "switch model", kind: builtinCmd},
		{insert: "/clear", label: "/clear", detail: "clear scrollback", kind: builtinCmd},
		{insert: "/compact", label: "/compact", detail: "compact messages", kind: builtinCmd},
		{insert: "/deploy", label: "/deploy", detail: "ship the build", kind: customCmd},
		{insert: "/diagnose", label: "/diagnose", detail: "bug hunt", kind: skillCmd},
	}
}

// TestOpenPopulatesFilteredWithAll verifies Open seeds the filtered set with
// every item (in items order) and resets the cursor to the top.
func TestOpenPopulatesFilteredWithAll(t *testing.T) {
	var c completions
	items := sampleItems()
	c.Open('/', items, 3)

	if !c.Active() {
		t.Fatal("Open should activate the popup")
	}
	if c.Trigger() != '/' {
		t.Fatalf("Trigger = %q, want '/'", c.Trigger())
	}
	if c.AnchorStart() != 3 {
		t.Fatalf("AnchorStart = %d, want 3", c.AnchorStart())
	}
	if len(c.filtered) != len(items) {
		t.Fatalf("filtered len = %d, want %d", len(c.filtered), len(items))
	}
	for i := range items {
		if c.filtered[i] != i {
			t.Fatalf("filtered[%d] = %d, want %d (Open should keep items order)", i, c.filtered[i], i)
		}
	}
	if c.cursor != 0 {
		t.Fatalf("cursor = %d, want 0 after Open", c.cursor)
	}
	if got, ok := c.Selected(); !ok || got.insert != "/models" {
		t.Fatalf("Selected after Open = (%+v, %v), want first item", got, ok)
	}
}

// TestSetQueryNarrows verifies the query fuzzy-filters the set and that the
// survivors are exactly the matching labels.
func TestSetQueryNarrows(t *testing.T) {
	var c completions
	c.Open('/', sampleItems(), 0)

	c.SetQuery("comp")
	if len(c.filtered) != 1 {
		t.Fatalf("SetQuery(comp) filtered len = %d, want 1", len(c.filtered))
	}
	if got, _ := c.Selected(); got.insert != "/compact" {
		t.Fatalf("SetQuery(comp) selected = %q, want /compact", got.insert)
	}

	// "d" matches /models, /compact (mid-word d), /deploy, /diagnose — every
	// label containing a 'd'. The narrowing is real: /clear has no 'd'.
	c.SetQuery("d")
	for _, fi := range c.filtered {
		if !strings.ContainsRune(strings.ToLower(c.items[fi].label), 'd') {
			t.Fatalf("SetQuery(d) kept %q which has no 'd'", c.items[fi].label)
		}
	}
	if len(c.filtered) >= len(c.items) {
		t.Fatalf("SetQuery(d) should drop /clear, got %d of %d", len(c.filtered), len(c.items))
	}
	if c.cursor != 0 {
		t.Fatalf("SetQuery should reset cursor to 0, got %d", c.cursor)
	}
}

// TestSetQuerySortsByScoreThenLabel pins the ordering contract: a prefix match
// outranks a mid-word match, and ties break lexicographically by label.
func TestSetQuerySortsByScoreThenLabel(t *testing.T) {
	var c completions
	c.Open('/', sampleItems(), 0)

	// "c" prefix-matches /clear and /compact (tier 1); it also subsequence-
	// matches /diagnose ('c' mid-word, tier 3). Prefix matches sort first, and
	// between the two prefix matches /clear < /compact lexicographically.
	c.SetQuery("c")
	if len(c.filtered) < 2 {
		t.Fatalf("SetQuery(c) filtered len = %d, want >= 2", len(c.filtered))
	}
	if got := c.items[c.filtered[0]].label; got != "/clear" {
		t.Fatalf("first match = %q, want /clear", got)
	}
	if got := c.items[c.filtered[1]].label; got != "/compact" {
		t.Fatalf("second match = %q, want /compact", got)
	}
}

// TestUpDownWrapAndClamp verifies cursor navigation wraps at both ends and
// clamps to the filtered set rather than the full item list.
func TestUpDownWrapAndClamp(t *testing.T) {
	var c completions
	c.Open('/', sampleItems(), 0)
	n := len(c.filtered)

	// Down from the top advances; Down off the bottom wraps to the top.
	c.Down()
	if c.cursor != 1 {
		t.Fatalf("after one Down, cursor = %d, want 1", c.cursor)
	}
	for i := 1; i < n; i++ {
		c.Down()
	}
	if c.cursor != 0 {
		t.Fatalf("Down off the bottom should wrap to 0, got %d", c.cursor)
	}

	// Up from the top wraps to the last row.
	c.Up()
	if c.cursor != n-1 {
		t.Fatalf("Up off the top should wrap to %d, got %d", n-1, c.cursor)
	}

	// After narrowing, the cursor must stay within the smaller filtered set.
	c.SetQuery("comp") // exactly one match
	c.Down()           // wrap onto itself
	if c.cursor != 0 {
		t.Fatalf("single-match Down should stay at 0, got %d", c.cursor)
	}
	c.Up()
	if c.cursor != 0 {
		t.Fatalf("single-match Up should stay at 0, got %d", c.cursor)
	}
}

// TestSelectedReturnsCursorItem verifies Selected follows the cursor and
// reports false when inactive or when nothing matches.
func TestSelectedReturnsCursorItem(t *testing.T) {
	var c completions
	if _, ok := c.Selected(); ok {
		t.Fatal("Selected on a closed popup should report false")
	}

	c.Open('/', sampleItems(), 0)
	c.Down()
	c.Down()
	got, ok := c.Selected()
	if !ok {
		t.Fatal("Selected should report true with matches")
	}
	if got.insert != c.items[c.filtered[2]].insert {
		t.Fatalf("Selected = %q, want the cursor item %q", got.insert, c.items[c.filtered[2]].insert)
	}

	c.SetQuery("zzz-no-such-command")
	if _, ok := c.Selected(); ok {
		t.Fatal("Selected with zero matches should report false")
	}
}

// TestViewEmptyWhenInactiveOrNoMatch verifies View renders nothing in the two
// "no popup" states, and Lines agrees.
func TestViewEmptyWhenInactiveOrNoMatch(t *testing.T) {
	th := DarkTheme()
	var c completions

	if v := c.View(th, 80); v != "" {
		t.Fatalf("View on a closed popup = %q, want empty", v)
	}
	if c.Lines() != 0 {
		t.Fatalf("Lines on a closed popup = %d, want 0", c.Lines())
	}

	c.Open('/', sampleItems(), 0)
	c.SetQuery("zzz-no-match")
	if v := c.View(th, 80); v != "" {
		t.Fatalf("View with zero matches = %q, want empty", v)
	}
	if c.Lines() != 0 {
		t.Fatalf("Lines with zero matches = %d, want 0", c.Lines())
	}
}

// TestViewNonEmptyAndLinesMatch verifies that an active popup with matches
// renders a non-empty box, Lines > 0, and Lines equals the rendered row count
// (newlines + 1).
func TestViewNonEmptyAndLinesMatch(t *testing.T) {
	th := DarkTheme()
	var c completions
	c.Open('/', sampleItems(), 0)

	v := c.View(th, 80)
	if v == "" {
		t.Fatal("View with matches should be non-empty")
	}
	if c.Lines() <= 0 {
		t.Fatalf("Lines with matches = %d, want > 0", c.Lines())
	}

	gotLines := strings.Count(v, "\n") + 1
	if gotLines != c.Lines() {
		t.Fatalf("rendered line count = %d, Lines() = %d (must agree)\n--- view ---\n%s", gotLines, c.Lines(), v)
	}
}

// TestViewCapsAtTenRowsAndScrolls verifies that with more than complMaxRows
// matches the popup shows exactly the cap (plus the two border rows) and that
// Lines tracks the rendered output even while scrolling.
func TestViewCapsAtTenRowsAndScrolls(t *testing.T) {
	th := DarkTheme()
	var c completions

	items := make([]complItem, 25)
	for i := range items {
		name := "/cmd" + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26)) + itoa2(i)
		items[i] = complItem{insert: name, label: name, detail: "row", kind: builtinCmd}
	}
	c.Open('/', items, 0)

	if c.visibleRows() != complMaxRows {
		t.Fatalf("visibleRows = %d, want %d", c.visibleRows(), complMaxRows)
	}
	if want := complMaxRows + 2; c.Lines() != want {
		t.Fatalf("Lines = %d, want %d (cap + 2 borders)", c.Lines(), want)
	}

	v := c.View(th, 80)
	if got := strings.Count(v, "\n") + 1; got != c.Lines() {
		t.Fatalf("rendered lines = %d, Lines() = %d", got, c.Lines())
	}

	// Scroll deep into the list; the line count is stable and still agrees.
	for i := 0; i < 20; i++ {
		c.Down()
	}
	v = c.View(th, 80)
	if got := strings.Count(v, "\n") + 1; got != c.Lines() {
		t.Fatalf("after scrolling, rendered lines = %d, Lines() = %d", got, c.Lines())
	}
}

// TestSetMaxRowsClampsHeight verifies the layout's terminal-height ceiling
// bounds the popup: visibleRows, Lines, and View all shrink in lockstep, a cap
// of 0 suppresses the popup entirely, and a cap above the match set is inert.
// This is the regression guard for the short-terminal overflow that pushed the
// input box off-screen.
func TestSetMaxRowsClampsHeight(t *testing.T) {
	th := DarkTheme()

	// 25 candidates: without a cap the popup shows the complMaxRows window.
	items := make([]complItem, 25)
	for i := range items {
		name := "/cmd" + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26)) + itoa2(i)
		items[i] = complItem{insert: name, label: name, detail: "row", kind: builtinCmd}
	}

	// A cap below the complMaxRows window wins; Lines() = cap + 2 borders and the
	// rendered box agrees.
	var c completions
	c.Open('/', items, 0)
	c.SetMaxRows(3)
	if got := c.visibleRows(); got != 3 {
		t.Fatalf("visibleRows under cap=3 = %d, want 3", got)
	}
	if want := 3 + 2; c.Lines() != want {
		t.Fatalf("Lines under cap=3 = %d, want %d (cap + 2 borders)", c.Lines(), want)
	}
	if got := strings.Count(c.View(th, 80), "\n") + 1; got != c.Lines() {
		t.Fatalf("rendered lines under cap = %d, Lines() = %d (must agree)", got, c.Lines())
	}

	// A cap of 0 (no room above the input) suppresses the popup: 0 rows, Lines
	// 0, View empty — never a stray bordered box that would still overflow.
	c.SetMaxRows(0)
	if got := c.visibleRows(); got != 0 {
		t.Fatalf("visibleRows under cap=0 = %d, want 0", got)
	}
	if c.Lines() != 0 {
		t.Fatalf("Lines under cap=0 = %d, want 0", c.Lines())
	}
	if v := c.View(th, 80); v != "" {
		t.Fatalf("View under cap=0 = %q, want empty", v)
	}

	// A cap at or above the complMaxRows window is inert: the window still wins.
	c.SetMaxRows(complMaxRows + 5)
	if got := c.visibleRows(); got != complMaxRows {
		t.Fatalf("visibleRows under generous cap = %d, want %d", got, complMaxRows)
	}
}

// TestViewBoldsMatchedRunes verifies a filtered query produces bold SGR around
// the matched runes in a non-cursor row's label.
func TestViewBoldsMatchedRunes(t *testing.T) {
	th := DarkTheme()
	var c completions
	c.Open('/', sampleItems(), 0)
	// "c" matches several labels (/clear, /compact, /diagnose). The cursor sits
	// on the first match and uses the selection band; the remaining non-cursor
	// rows bold their matched runes, so bold SGR must appear from a per-rune
	// highlight, not just the band.
	c.SetQuery("c")
	if len(c.filtered) < 2 {
		t.Fatalf("SetQuery(c) should match multiple rows, got %d", len(c.filtered))
	}

	v := c.View(th, 80)
	if v == "" {
		t.Fatal("View should render for a matching query")
	}
	if !strings.Contains(v, "\x1b[1m") {
		t.Fatalf("View should contain bold SGR for matched runes:\n%s", v)
	}
}

// TestCloseResetsState verifies Close clears the popup so a stale candidate set
// can never render afterward.
func TestCloseResetsState(t *testing.T) {
	var c completions
	c.Open('/', sampleItems(), 7)
	c.Close()

	if c.Active() {
		t.Fatal("Close should deactivate the popup")
	}
	if c.items != nil || c.filtered != nil {
		t.Fatal("Close should drop the candidate state")
	}
	if v := c.View(DarkTheme(), 80); v != "" {
		t.Fatalf("View after Close = %q, want empty", v)
	}
}

// itoa2 renders i as a fixed-ish suffix so generated names stay unique without
// pulling in strconv for a test fixture.
func itoa2(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return string(rune('0'+i/10)) + string(rune('0'+i%10))
}
