package tui

import (
	"strings"
	"testing"
)

const sampleUnified = `@@ -1,3 +1,3 @@
 context line
-old line
+new line
 trailing`

func TestRenderDiffAuto_WideUsesSplit(t *testing.T) {
	th := DarkTheme()
	out := renderDiffAuto(th, sampleUnified, "go", 160)
	plain := stripANSI(out)
	// In split mode, context lines appear on both sides of the same row,
	// so "context line" should occur twice on a single output line.
	foundCtxBoth := false
	for _, ln := range strings.Split(plain, "\n") {
		if strings.Count(ln, "context line") >= 2 {
			foundCtxBoth = true
		}
	}
	if !foundCtxBoth {
		t.Fatalf("wide width should render split (context line on both sides):\n%s", plain)
	}
	// Del and add lines should be on separate rows (not unified).
	hasDelRow := false
	hasAddRow := false
	for _, ln := range strings.Split(plain, "\n") {
		if strings.Contains(ln, "old line") && !strings.Contains(ln, "new line") {
			hasDelRow = true
		}
		if strings.Contains(ln, "new line") && !strings.Contains(ln, "old line") {
			hasAddRow = true
		}
	}
	if !hasDelRow || !hasAddRow {
		t.Fatalf("wide split should have separate del/add rows: del=%v add=%v\n%s", hasDelRow, hasAddRow, plain)
	}
}

func TestRenderDiffAuto_NarrowUsesUnified(t *testing.T) {
	th := DarkTheme()
	out := renderDiffAuto(th, sampleUnified, "go", 60)
	for _, ln := range strings.Split(out, "\n") {
		if strings.Contains(ln, "old line") && strings.Contains(ln, "new line") {
			t.Fatalf("narrow width must stay unified, got split row:\n%s", out)
		}
	}
}
