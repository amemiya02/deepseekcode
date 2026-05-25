package tui

import (
	"strings"
	"testing"
)

func TestWelcome(t *testing.T) {
	th := DarkTheme()

	t.Run("wide terminal", func(t *testing.T) {
		out := renderWelcome(th, 100)
		if !strings.Contains(out, "\x1b[") {
			t.Fatal("expected ANSI color sequences in wide output")
		}
		plain := stripANSI(out)
		// Should contain the diagonal field, letterform glyphs, brand, tagline.
		if !strings.Contains(plain, "╱") {
			t.Fatal("expected diagonal field character in output")
		}
		if !strings.Contains(plain, "█") {
			t.Fatal("expected letterform block character in output")
		}
		if !strings.Contains(plain, "deepseekcode") {
			t.Fatal("expected brand in meta row")
		}
		if !strings.Contains(plain, "terminal coding agent") {
			t.Fatal("expected tagline in output")
		}
	})

	t.Run("narrow terminal falls back", func(t *testing.T) {
		out := renderWelcome(th, 40)
		if strings.Contains(out, "╱") {
			t.Fatal("narrow output should not contain logo field")
		}
		if !strings.Contains(out, "deepseekcode") {
			t.Fatal("expected single-line fallback")
		}
		if !strings.Contains(out, "/help") {
			t.Fatal("expected /help hint in narrow output")
		}
	})
}
