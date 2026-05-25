package tui

import (
	"strings"
	"testing"
)

func TestHighlight(t *testing.T) {
	th := DarkTheme()

	t.Run("Go code", func(t *testing.T) {
		src := "package main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"
		out := Highlight(th, src, "go")
		if !strings.Contains(out, "\x1b[") {
			t.Fatal("expected ANSI color sequences in highlighted output")
		}
		if !strings.Contains(out, "func") {
			t.Fatal("expected 'func' token in output")
		}
		if !strings.Contains(out, "main") {
			t.Fatal("expected 'main' token in output")
		}
	})

	t.Run("bash code", func(t *testing.T) {
		src := "echo hello"
		out := Highlight(th, src, "bash")
		if !strings.Contains(out, "\x1b[") {
			t.Fatal("expected ANSI color sequences")
		}
		if !strings.Contains(out, "echo") {
			t.Fatal("expected 'echo' token in output")
		}
	})

	t.Run("unknown language returns source unchanged", func(t *testing.T) {
		src := "weird ¿¿ content"
		out := Highlight(th, src, "")
		if out != src {
			t.Fatalf("expected source unchanged, got %q", out)
		}
	})

	t.Run("empty source returns empty", func(t *testing.T) {
		out := Highlight(th, "", "go")
		if out != "" {
			t.Fatalf("expected empty string, got %q", out)
		}
	})
}
