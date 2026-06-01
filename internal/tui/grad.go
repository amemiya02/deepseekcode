package tui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/rivo/uniseg"
)

// ForegroundGrad returns a slice of strings representing the input string
// rendered with a horizontal gradient foreground from color1 to color2. Each
// string in the returned slice corresponds to a grapheme cluster in the input
// string.
func ForegroundGrad(base lipgloss.Style, input string, bold bool, color1, color2 color.Color) []string {
	if input == "" {
		return []string{""}
	}
	if len(input) == 1 {
		style := base.Foreground(color1)
		if bold {
			style = style.Bold(true)
		}
		return []string{style.Render(input)}
	}
	var clusters []string
	gr := uniseg.NewGraphemes(input)
	for gr.Next() {
		clusters = append(clusters, string(gr.Runes()))
	}

	ramp := lipgloss.Blend1D(len(clusters), color1, color2)
	for i, c := range ramp {
		style := base.Foreground(c)
		if bold {
			style = style.Bold(true)
		}
		clusters[i] = style.Render(clusters[i])
	}
	return clusters
}

// ApplyForegroundGrad renders a given string with a horizontal gradient
// foreground. Returns "" for empty input.
func ApplyForegroundGrad(base lipgloss.Style, input string, color1, color2 color.Color) string {
	if input == "" {
		return ""
	}
	var o strings.Builder
	clusters := ForegroundGrad(base, input, false, color1, color2)
	for _, c := range clusters {
		fmt.Fprint(&o, c)
	}
	return o.String()
}

// ApplyBoldForegroundGrad renders a given string with a bold horizontal
// gradient foreground. Returns "" for empty input.
func ApplyBoldForegroundGrad(base lipgloss.Style, input string, color1, color2 color.Color) string {
	if input == "" {
		return ""
	}
	var o strings.Builder
	clusters := ForegroundGrad(base, input, true, color1, color2)
	for _, c := range clusters {
		fmt.Fprint(&o, c)
	}
	return o.String()
}

// ApplyForegroundGradPhase renders input with a phase-offset gradient: the
// gradient sweep starts at grapheme index (phase % len) instead of 0, creating
// a "shimmer" effect when phase advances each tick. The ramp is generated
// from color1→color2 via Blend1D; the phase offset wraps via modulo.
func ApplyForegroundGradPhase(base lipgloss.Style, input string, color1, color2 color.Color, phase int) string {
	if input == "" {
		return ""
	}
	var clusters []string
	gr := uniseg.NewGraphemes(input)
	for gr.Next() {
		clusters = append(clusters, string(gr.Runes()))
	}
	n := len(clusters)
	if n == 0 {
		return ""
	}

	// Build a ramp: color1 → color2, one entry per grapheme.
	ramp := lipgloss.Blend1D(n, color1, color2)

	// Phase-shift: rotate the ramp so the gradient sweep starts at the
	// grapheme index indicated by phase. This creates a living shimmer as
	// phase advances each tick.
	shift := phase % n
	var o strings.Builder
	for i, cl := range clusters {
		c := ramp[(i+shift)%n]
		style := base.Foreground(c)
		fmt.Fprint(&o, style.Render(cl))
	}
	return o.String()
}
