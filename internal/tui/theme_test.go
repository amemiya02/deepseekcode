package tui

import (
	"testing"

	"charm.land/lipgloss/v2"
)

func TestTheme(t *testing.T) {
	t.Run("DarkTheme Ocean colors", func(t *testing.T) {
		th := DarkTheme()
		if th.Name != "dark" {
			t.Fatalf("expected name 'dark', got %q", th.Name)
		}
		if th.BrandDeep != lipgloss.Color("#4d7fe0") {
			t.Fatalf("expected BrandDeep #4d7fe0, got %v", th.BrandDeep)
		}
		if th.BrandLight != lipgloss.Color("#8fc7ff") {
			t.Fatalf("expected BrandLight #8fc7ff, got %v", th.BrandLight)
		}
		// The selection pair must be populated and distinct from the brand:
		// the focused-row band is a recessive deep indigo, not the bright brand.
		if th.SelBg != lipgloss.Color("#2c4a7d") {
			t.Fatalf("expected SelBg #2c4a7d, got %v", th.SelBg)
		}
		if th.SelFg != lipgloss.Color("#eaf1fc") {
			t.Fatalf("expected SelFg #eaf1fc, got %v", th.SelFg)
		}
	})

	t.Run("DarkTheme all fields non-zero", func(t *testing.T) {
		th := DarkTheme()
		if th.UserPrompt.GetForeground() == nil {
			t.Fatal("UserPrompt foreground is nil")
		}
		if th.ToolCall.GetForeground() == nil {
			t.Fatal("ToolCall foreground is nil")
		}
		if th.ToolOk.GetForeground() == nil {
			t.Fatal("ToolOk foreground is nil")
		}
		if th.ToolErr.GetForeground() == nil {
			t.Fatal("ToolErr foreground is nil")
		}
		if th.Error.GetForeground() == nil {
			t.Fatal("Error foreground is nil")
		}
		if th.StatusModel.GetForeground() == nil {
			t.Fatal("StatusModel foreground is nil")
		}
	})

	t.Run("LightTheme name", func(t *testing.T) {
		th := LightTheme()
		if th.Name != "light" {
			t.Fatalf("expected name 'light', got %q", th.Name)
		}
	})

	t.Run("Card styles present", func(t *testing.T) {
		th := DarkTheme()
		if th.CardBar.GetForeground() == nil {
			t.Fatal("CardBar foreground is nil")
		}
		if th.CardHeader.GetForeground() == nil {
			t.Fatal("CardHeader foreground is nil")
		}
		if th.CardBody.GetForeground() == nil {
			t.Fatal("CardBody foreground is nil")
		}
	})

	t.Run("PickTheme defaults to dark", func(t *testing.T) {
		th := PickTheme("unknown")
		if th.Name != "dark" {
			t.Fatalf("expected 'dark', got %q", th.Name)
		}
		th2 := PickTheme("light")
		if th2.Name != "light" {
			t.Fatalf("expected 'light', got %q", th2.Name)
		}
	})
}
