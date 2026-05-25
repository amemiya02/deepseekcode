package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

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
	if !strings.Contains(plain, "deepseekcode") || !strings.Contains(plain, "v0.1") {
		t.Error("expected brand + version in meta row")
	}
	// 4 rows (meta + 3 wordmark rows), each newline-terminated.
	if n := strings.Count(out, "\n"); n != 4 {
		t.Errorf("expected 4 lines, got %d", n)
	}
	// No rendered line may exceed the requested width.
	for _, line := range strings.Split(strings.TrimRight(plain, "\n"), "\n") {
		if w := lipgloss.Width(line); w > 100 {
			t.Errorf("line exceeds width 100: %d (%q)", w, line)
		}
	}
}

func TestRenderLogoNarrowDoesNotPanic(t *testing.T) {
	// renderLogo is only called for wide terminals, but it must not panic
	// if handed a small width (right field clamps to 0).
	_ = renderLogo(DarkTheme(), "deepseek", "deepseekcode", "v0.1", 50)
}
