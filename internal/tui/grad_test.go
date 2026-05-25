package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestForegroundGrad(t *testing.T) {
	base := lipgloss.NewStyle()
	deep := lipgloss.Color("#1D4ED8")
	light := lipgloss.Color("#7DD3FC")

	t.Run("two chars", func(t *testing.T) {
		clusters := ForegroundGrad(base, "ab", false, deep, light)
		if len(clusters) != 2 {
			t.Fatalf("expected 2 clusters, got %d", len(clusters))
		}
	})

	t.Run("empty string", func(t *testing.T) {
		clusters := ForegroundGrad(base, "", false, deep, light)
		if len(clusters) != 1 || clusters[0] != "" {
			t.Fatalf("expected [\"\"], got %v", clusters)
		}
	})

	t.Run("single char", func(t *testing.T) {
		clusters := ForegroundGrad(base, "X", false, deep, light)
		if len(clusters) != 1 {
			t.Fatalf("expected 1 cluster, got %d", len(clusters))
		}
		if !strings.Contains(clusters[0], "X") {
			t.Fatalf("expected output to contain 'X', got %q", clusters[0])
		}
	})

	t.Run("ANSI color present", func(t *testing.T) {
		clusters := ForegroundGrad(base, "DEEP", false, deep, light)
		for _, c := range clusters {
			if !strings.Contains(c, "\x1b[") {
				t.Fatalf("expected ANSI color sequence in %q", c)
			}
		}
	})
}

func TestApplyForegroundGrad(t *testing.T) {
	base := lipgloss.NewStyle()
	deep := lipgloss.Color("#1D4ED8")
	light := lipgloss.Color("#7DD3FC")

	t.Run("non-empty", func(t *testing.T) {
		result := ApplyForegroundGrad(base, "DEEPSEEKCODE", deep, light)
		if result == "" {
			t.Fatal("expected non-empty result")
		}
		if !strings.Contains(result, "\x1b[") {
			t.Fatalf("expected ANSI color sequence, got %q", result)
		}
	})

	t.Run("empty string", func(t *testing.T) {
		result := ApplyForegroundGrad(base, "", deep, light)
		if result != "" {
			t.Fatalf("expected empty string, got %q", result)
		}
	})
}

func TestApplyBoldForegroundGrad(t *testing.T) {
	base := lipgloss.NewStyle()
	deep := lipgloss.Color("#1D4ED8")
	light := lipgloss.Color("#7DD3FC")

	t.Run("non-empty", func(t *testing.T) {
		result := ApplyBoldForegroundGrad(base, "DSC", deep, light)
		if result == "" {
			t.Fatal("expected non-empty result")
		}
		if !strings.Contains(result, "\x1b[") {
			t.Fatalf("expected ANSI color sequence, got %q", result)
		}
	})

	t.Run("empty string", func(t *testing.T) {
		result := ApplyBoldForegroundGrad(base, "", deep, light)
		if result != "" {
			t.Fatalf("expected empty string, got %q", result)
		}
	})

	t.Run("bold differs from non-bold", func(t *testing.T) {
		bold := ApplyBoldForegroundGrad(base, "DSC", deep, light)
		plain := ApplyForegroundGrad(base, "DSC", deep, light)
		if bold == plain {
			t.Fatal("bold and non-bold output must differ")
		}
	})
}
