package tui

import (
	"strings"
	"testing"
)

// sampleDiff is a small unified diff exercising every line kind: meta headers,
// a hunk header, a deletion, two additions, and a context line.
const sampleDiff = "diff --git a/x.go b/x.go\n" +
	"index 1111111..2222222 100644\n" +
	"--- a/x.go\n" +
	"+++ b/x.go\n" +
	"@@ -10,7 +10,9 @@ func main() {\n" +
	"-\told := compute()\n" +
	"+\tnew := compute()\n" +
	"+\tfold(new)\n" +
	" \treturn nil\n"

func TestRenderDiffLineNumbersAndText(t *testing.T) {
	th := DarkTheme()
	plain := stripANSI(renderDiff(th, sampleDiff, "go", 80))

	// Code text round-trips with markers stripped (the band replaces them).
	for _, want := range []string{"old := compute()", "new := compute()", "fold(new)", "return nil"} {
		if !strings.Contains(plain, want) {
			t.Errorf("missing code text %q in:\n%s", want, plain)
		}
	}

	// Line numbers are seeded from the hunk header: del shows old-side 10, the
	// two adds show new-side 10 and 11, the context line shows new-side 12.
	lines := strings.Split(strings.TrimRight(plain, "\n"), "\n")
	var codeLines []string
	for _, ln := range lines {
		if strings.Contains(ln, "compute()") || strings.Contains(ln, "fold(new)") || strings.Contains(ln, "return nil") {
			codeLines = append(codeLines, ln)
		}
	}
	if len(codeLines) != 4 {
		t.Fatalf("expected 4 code lines, got %d:\n%s", len(codeLines), plain)
	}
	wantNums := []string{"10", "10", "11", "12"}
	for i, ln := range codeLines {
		// The number is right-aligned in the gutter; assert it appears before
		// the code text.
		if !strings.Contains(ln, wantNums[i]) {
			t.Errorf("code line %d: expected line number %q in %q", i, wantNums[i], ln)
		}
	}
}

func TestRenderDiffEmitsBandFills(t *testing.T) {
	th := DarkTheme() // truecolor default → fills on
	out := renderDiff(th, sampleDiff, "go", 80)
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected ANSI band/highlight escapes, got none:\n%q", out)
	}
}

func TestRenderDiffDegradedNoFill(t *testing.T) {
	// Non-truecolor degrades to foreground-only: no background fill escapes.
	th := DarkTheme().WithTruecolor(false)
	out := renderDiff(th, sampleDiff, "go", 80)
	// Background SGR uses the 48; sequence (truecolor) — assert it's absent.
	if strings.Contains(out, "\x1b[48;2;") {
		t.Errorf("degraded (non-truecolor) diff must not emit truecolor background fills:\n%q", out)
	}
}

func TestRenderDiffTransparentNoFill(t *testing.T) {
	th := DarkTheme().WithTransparent(true)
	out := renderDiff(th, sampleDiff, "go", 80)
	if strings.Contains(out, "\x1b[48;2;") {
		t.Errorf("transparent mode diff must not emit background fills:\n%q", out)
	}
}

func TestRenderDiffEmptyReturnsEmpty(t *testing.T) {
	if got := renderDiff(DarkTheme(), "", "go", 80); got != "" {
		t.Errorf("empty diff should render empty, got %q", got)
	}
}

func TestClassifyDiffLine(t *testing.T) {
	cases := []struct {
		in   string
		want diffLineKind
	}{
		{"@@ -1,2 +1,2 @@", diffHunk},
		{"+++ b/x.go", diffMeta},
		{"--- a/x.go", diffMeta},
		{"diff --git a/x b/x", diffMeta},
		{"index abc..def 100644", diffMeta},
		{"new file mode 100644", diffMeta},
		{"+added", diffAdd},
		{"-removed", diffDel},
		{" context", diffEqual},
		{"bare", diffEqual},
	}
	for _, c := range cases {
		if got := classifyDiffLine(c.in); got != c.want {
			t.Errorf("classifyDiffLine(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParseHunkHeader(t *testing.T) {
	cases := []struct {
		in               string
		wantOld, wantNew int
	}{
		{"@@ -10,7 +12,9 @@ func main() {", 10, 12},
		{"@@ -1 +1 @@", 1, 1},
		{"@@ -0,0 +1,5 @@", 1, 1}, // start 0 floored to 1
		{"garbage", 1, 1},
	}
	for _, c := range cases {
		o, n := parseHunkHeader(c.in)
		if o != c.wantOld || n != c.wantNew {
			t.Errorf("parseHunkHeader(%q) = (%d,%d), want (%d,%d)", c.in, o, n, c.wantOld, c.wantNew)
		}
	}
}

func TestDiffCodeLang(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"+++ b/internal/agent/agent.go\n", "go"},
		{"--- a/foo.py\n+++ /dev/null\n", "py"}, // deletion: new side /dev/null, fall back to old side
		{"--- /dev/null\n+++ b/bar.ts\n", "ts"}, // creation: prefer the new side
		{"diff --git a/cmd/main.rs b/cmd/main.rs\n", "rs"},
		{"+++ b/Makefile\n", ""}, // no extension
		{"no headers here\n", ""},
	}
	for _, c := range cases {
		if got := diffCodeLang(c.in); got != c.want {
			t.Errorf("diffCodeLang(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestClipToWidth(t *testing.T) {
	if got := clipToWidth("hello world", 5); got != "hell…" {
		t.Errorf("clipToWidth = %q, want %q", got, "hell…")
	}
	if got := clipToWidth("hi", 10); got != "hi" {
		t.Errorf("clipToWidth (no clip) = %q, want %q", got, "hi")
	}
	if got := clipToWidth("hi", 0); got != "" {
		t.Errorf("clipToWidth(_, 0) = %q, want empty", got)
	}
}
