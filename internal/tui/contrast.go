package tui

import (
	"image/color"
	"math"
)

// srgbToLinear converts a single sRGB channel value in [0,1] to linear.
func srgbToLinear(c float64) float64 {
	if c <= 0.03928 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

// colorToRGBA converts any color.Color to color.RGBA using the standard
// RGBA() method (returns pre-multiplied 16-bit values → 8-bit).
func colorToRGBA(c color.Color) color.Color {
	r, g, b, a := c.RGBA()
	return color.RGBA{
		R: uint8(r >> 8),
		G: uint8(g >> 8),
		B: uint8(b >> 8),
		A: uint8(a >> 8),
	}
}

// relativeLuminance returns WCAG 2.x relative luminance in [0,1].
// See https://www.w3.org/TR/WCAG21/#dfn-relative-luminance.
func relativeLuminance(c color.Color) float64 {
	rgba := colorToRGBA(c)
	r := srgbToLinear(float64(rgba.(color.RGBA).R) / 255.0)
	g := srgbToLinear(float64(rgba.(color.RGBA).G) / 255.0)
	b := srgbToLinear(float64(rgba.(color.RGBA).B) / 255.0)
	return 0.2126*r + 0.7152*g + 0.0722*b
}

// contrastRatio returns the WCAG contrast ratio in [1,21] between fg and bg.
// See https://www.w3.org/TR/WCAG21/#dfn-contrast-ratio.
func contrastRatio(fg, bg color.Color) float64 {
	l1 := relativeLuminance(fg)
	l2 := relativeLuminance(bg)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}
