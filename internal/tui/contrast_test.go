package tui

import (
	"image/color"
	"testing"

	"charm.land/lipgloss/v2"
)

// pairing names a text-on-background color pairing used in a theme.
type pairing struct {
	label string
	fg    color.Color
	bg    color.Color
	// largeText is true when the theme renders this pairing bold or ≥24px
	// equivalent (e.g. headers, badges). WCAG AA requires only 3:1 for large text.
	largeText bool
}

// themePairings returns the 9 text pairings actually used in each theme,
// drawn from the palette tokens as the card specifies.
func themePairings(p palette) []pairing {
	return []pairing{
		{"fgBase-on-bgBase", p.fgBase, p.bgBase, false},
		{"fgMuted-on-bgSurface", p.fgMuted, p.bgSurface, false},
		{"fgSubtle-on-bgRaised", p.fgSubtle, p.bgRaised, false},
		{"fgFaint-on-bgRaised", p.fgFaint, p.bgRaised, false},
		{"onAccent-on-brandDeep", p.onAccent, p.brandDeep, true},
		{"onAccent-on-accentPro", p.onAccent, p.accentPro, true},
		{"ok-on-bgSurface", p.ok, p.bgSurface, false},
		{"errc-on-bgSurface", p.errc, p.bgSurface, false},
		{"warn-on-bgSurface", p.warn, p.bgSurface, false},
	}
}

func TestContrastRatioBasics(t *testing.T) {
	white := color.RGBA{255, 255, 255, 255}
	black := color.RGBA{0, 0, 0, 255}

	ratio := contrastRatio(white, black)
	if ratio < 20.9 || ratio > 21.1 {
		t.Errorf("contrastRatio(white,black) = %.2f, want 21±0.1", ratio)
	}

	ratio = contrastRatio(white, white)
	if ratio != 1.0 {
		t.Errorf("contrastRatio(white,white) = %.2f, want 1.0", ratio)
	}
}

func TestContrastGate(t *testing.T) {
	themes := []struct {
		name string
		pal  palette
	}{
		{"dark", oceanPalette},
		{"light", lightPalette},
		{"midnight", midnightPalette},
	}

	// Verify we tested all expected pairings: 9 pairings × 3 themes = 27 checks.
	total := 0
	for _, th := range themes {
		total += len(themePairings(th.pal))
	}
	if total < 27 {
		t.Errorf("expected ≥27 pairing checks, got %d", total)
	}

	// All pairings must hard-pass AA (4.5:1 text / 3:1 large text).
	for _, th := range themes {
		pairs := themePairings(th.pal)
		for _, p := range pairs {
			ratio := contrastRatio(p.fg, p.bg)
			threshold := 4.5
			qual := "AA"
			if p.largeText {
				threshold = 3.0
				qual = "AA-large"
			}
			if ratio < threshold {
				t.Errorf("%s / %s: %s ratio=%.2f < %.1f",
					th.name, p.label, qual, ratio, threshold)
			}
		}
	}
}

func TestRelativeLuminance(t *testing.T) {
	white := color.RGBA{255, 255, 255, 255}
	black := color.RGBA{0, 0, 0, 255}

	lw := relativeLuminance(white)
	if lw < 0.99 || lw > 1.01 {
		t.Errorf("relativeLuminance(white) = %.4f, want ~1.0", lw)
	}
	lb := relativeLuminance(black)
	if lb != 0.0 {
		t.Errorf("relativeLuminance(black) = %.4f, want 0.0", lb)
	}
}

func TestColorToRGBA(t *testing.T) {
	c := lipgloss.Color("#ff8040")
	rgba := colorToRGBA(c).(color.RGBA)
	if rgba.R != 255 || rgba.G != 128 || rgba.B != 64 {
		t.Errorf("colorToRGBA(#ff8040) = %v, want {255 128 64 255}", rgba)
	}

	// Black.
	c2 := lipgloss.Color("#000000")
	rgba2 := colorToRGBA(c2).(color.RGBA)
	if rgba2.R != 0 || rgba2.G != 0 || rgba2.B != 0 {
		t.Errorf("colorToRGBA(#000000) = %v, want {0 0 0 255}", rgba2)
	}
}
