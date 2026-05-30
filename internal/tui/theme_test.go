package tui

import (
	"image/color"
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

func TestNewThemeConstructors(t *testing.T) {
	tests := []struct {
		name string
		fn   func() Theme
	}{
		{"midnight", MidnightTheme},
		{"nebula", NebulaTheme},
		{"aurora", AuroraTheme},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			th := tt.fn()
			if th.Name != tt.name {
				t.Fatalf("expected Name %q, got %q", tt.name, th.Name)
			}
			if th.IsLight() {
				t.Fatalf("expected IsLight()==false for %s", tt.name)
			}
		})
	}
}

func TestIsLightPolarity(t *testing.T) {
	if DarkTheme().IsLight() {
		t.Fatal("DarkTheme should not be light")
	}
	if !LightTheme().IsLight() {
		t.Fatal("LightTheme should be light")
	}
	if MidnightTheme().IsLight() {
		t.Fatal("MidnightTheme should not be light")
	}
	if NebulaTheme().IsLight() {
		t.Fatal("NebulaTheme should not be light")
	}
	if AuroraTheme().IsLight() {
		t.Fatal("AuroraTheme should not be light")
	}
}

func TestAllThemesComplete(t *testing.T) {
	themes := []Theme{DarkTheme(), LightTheme(), MidnightTheme(), NebulaTheme(), AuroraTheme()}
	for _, th := range themes {
		if th.Name == "" {
			t.Fatalf("theme has empty Name")
		}
		colors := []struct {
			name string
			c    color.Color
		}{
			{"BrandDeep", th.BrandDeep},
			{"BrandLight", th.BrandLight},
			{"AccentFlash", th.AccentFlash},
			{"AccentPro", th.AccentPro},
			{"SelBg", th.SelBg},
			{"SelFg", th.SelFg},
			{"BgBase", th.BgBase},
			{"BgWell", th.BgWell},
			{"BgSurface", th.BgSurface},
			{"BgRaised", th.BgRaised},
			{"FgBase", th.FgBase},
			{"FgMuted", th.FgMuted},
			{"FgSubtle", th.FgSubtle},
			{"FgFaint", th.FgFaint},
			{"BorderColor", th.BorderColor},
			{"OkColor", th.OkColor},
			{"ErrColor", th.ErrColor},
			{"WarnColor", th.WarnColor},
			{"OnAccent", th.OnAccent},
			{"DiffAddFg", th.DiffAddFg},
			{"DiffAddBg", th.DiffAddBg},
			{"DiffDelFg", th.DiffDelFg},
			{"DiffDelBg", th.DiffDelBg},
		}
		for _, c := range colors {
			if c.c == nil {
				t.Fatalf("%s: %s is nil", th.Name, c.name)
			}
		}
	}
}

func TestThemeNamesUnique(t *testing.T) {
	themes := []Theme{DarkTheme(), LightTheme(), MidnightTheme(), NebulaTheme(), AuroraTheme()}
	seen := make(map[string]bool)
	for _, th := range themes {
		if seen[th.Name] {
			t.Fatalf("duplicate theme name: %q", th.Name)
		}
		seen[th.Name] = true
	}
}

func TestNewThemesAreDarkPolarity(t *testing.T) {
	oceanBg := DarkTheme().BgBase
	for _, th := range []Theme{MidnightTheme(), NebulaTheme(), AuroraTheme()} {
		if th.IsLight() {
			t.Fatalf("%s should have dark polarity", th.Name)
		}
		if th.BgBase == oceanBg {
			t.Fatalf("%s bgBase should differ from Ocean's", th.Name)
		}
	}
}

func TestThemeRegistryConsistent(t *testing.T) {
	for _, row := range availableThemes() {
		th := themeByID(row.ID)
		if th.Name != row.ID {
			t.Errorf("themeByID(%q).Name = %q, want %q", row.ID, th.Name, row.ID)
		}
	}
}

func TestThemeByIDUnknownDefaultsDark(t *testing.T) {
	th := themeByID("nope")
	if th.Name != "dark" {
		t.Fatalf("expected 'dark', got %q", th.Name)
	}
	th2 := themeByID("")
	if th2.Name != "dark" {
		t.Fatalf("expected 'dark', got %q", th2.Name)
	}
}

func TestPickThemeParity(t *testing.T) {
	if PickTheme("light").Name != "light" {
		t.Fatal("PickTheme(light) should return light")
	}
	if PickTheme("midnight").Name != "midnight" {
		t.Fatal("PickTheme(midnight) should return midnight")
	}
	if PickTheme("xxx").Name != "dark" {
		t.Fatal("PickTheme(xxx) should default to dark")
	}
}

func TestAvailableThemesStructure(t *testing.T) {
	rows := availableThemes()
	if len(rows) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(rows))
	}
	want := []string{"dark", "light", "midnight", "nebula", "aurora"}
	for i, id := range want {
		if rows[i].ID != id {
			t.Errorf("row[%d].ID = %q, want %q", i, rows[i].ID, id)
		}
		if rows[i].Label == "" {
			t.Errorf("row[%d].Label is empty", i)
		}
		if rows[i].Desc == "" {
			t.Errorf("row[%d].Desc is empty", i)
		}
	}
}
