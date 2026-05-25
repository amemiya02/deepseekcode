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
		if th.BrandDeep != lipgloss.Color("#1D4ED8") {
			t.Fatalf("expected BrandDeep #1D4ED8, got %v", th.BrandDeep)
		}
		if th.BrandLight != lipgloss.Color("#7DD3FC") {
			t.Fatalf("expected BrandLight #7DD3FC, got %v", th.BrandLight)
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
