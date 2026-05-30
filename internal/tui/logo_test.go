package tui

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

var updateGolden = flag.Bool("update-golden", false, "update golden files")

func TestWordmarkRows(t *testing.T) {
	rows := wordmarkRows("deepseek")
	if rows[0] == "" || rows[1] == "" || rows[2] == "" {
		t.Fatal("wordmark rows should be non-empty")
	}
	// All three rows must share one display width so columns align.
	w0 := lipgloss.Width(rows[0])
	if lipgloss.Width(rows[1]) != w0 || lipgloss.Width(rows[2]) != w0 {
		t.Errorf("rows misaligned: %d/%d/%d", w0, lipgloss.Width(rows[1]), lipgloss.Width(rows[2]))
	}
	// Unknown runes are skipped, not rendered as blanks.
	if got := wordmarkRows("d!k"); strings.Contains(got[0], "!") {
		t.Error("unknown rune should be skipped")
	}
}

func TestRenderLogo(t *testing.T) {
	th := DarkTheme()
	out := renderLogo(th, "deepseek", "deepseekcode", "v0.1", 100)
	if !strings.Contains(out, "\x1b[") {
		t.Error("expected ANSI gradient sequences")
	}
	plain := stripANSI(out)
	if !strings.Contains(plain, "╱") {
		t.Error("expected diagonal field")
	}
	if strings.Contains(plain, "deepseekcode") || strings.Contains(plain, "v0.1") {
		t.Error("welcome logo should not repeat brand + version from header")
	}
	// 3 gradient wordmark rows, each newline-terminated.
	if n := strings.Count(out, "\n"); n != 3 {
		t.Errorf("expected 3 lines, got %d", n)
	}
	// No rendered line may exceed the requested width.
	for _, line := range strings.Split(strings.TrimRight(plain, "\n"), "\n") {
		if w := lipgloss.Width(line); w > 100 {
			t.Errorf("line exceeds width 100: %d (%q)", w, line)
		}
	}
}

func TestRenderHeaderBar(t *testing.T) {
	th := DarkTheme()
	for _, w := range []int{10, 40, 80, 120} {
		out := renderHeaderBar(th, "deepseekcode", "v0.3.2", w)
		// Must be exactly 1 line (no newline).
		if strings.Contains(out, "\n") {
			t.Errorf("width %d: header contains newline", w)
		}
		// Must not exceed requested width.
		if got := lipgloss.Width(out); got > w {
			t.Errorf("width %d: lipgloss.Width = %d", w, got)
		}
	}
}

func TestRenderHeaderBarNoVersion(t *testing.T) {
	th := DarkTheme()
	out := renderHeaderBar(th, "deepseekcode", "", 80)
	if strings.Contains(out, "\n") {
		t.Error("header with no version should be 1 line")
	}
	if lipgloss.Width(out) > 80 {
		t.Errorf("header exceeds width: %d", lipgloss.Width(out))
	}
}

func TestRenderHeaderBarNarrowWidth(t *testing.T) {
	th := DarkTheme()
	// Width 0 → empty.
	if got := renderHeaderBar(th, "deepseekcode", "v1", 0); got != "" {
		t.Errorf("width 0: got %q, want empty", got)
	}
	// Very narrow → should not panic.
	_ = renderHeaderBar(th, "deepseekcode", "v1", 3)
}

// TestRenderHeaderBarGolden compares renderHeaderBar output against golden
// files at widths 40, 80, and 120. Run with -update-golden to regenerate.
func TestRenderHeaderBarGolden(t *testing.T) {
	th := DarkTheme()
	for _, w := range []int{40, 80, 120} {
		got := renderHeaderBar(th, "deepseekcode", "v0.3.2", w)
		path := filepath.Join("testdata", "render", fmt.Sprintf("header_w%d.golden", w))

		if *updateGolden {
			os.MkdirAll(filepath.Dir(path), 0o755)
			if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
				t.Fatalf("write golden %s: %v", path, err)
			}
			continue
		}

		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read golden %s: %v (run with -update-golden)", path, err)
		}
		if got != string(want) {
			t.Errorf("width %d: golden mismatch\ngot=%q\nwant=%q", w, got, string(want))
		}
	}
}

func TestRenderLogoNarrowDoesNotPanic(t *testing.T) {
	// renderLogo is only called for wide terminals, but it must not panic
	// if handed a small width (right field clamps to 0).
	_ = renderLogo(DarkTheme(), "deepseek", "deepseekcode", "v0.1", 50)
}
