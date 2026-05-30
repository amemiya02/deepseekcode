// logo.go renders the startup wordmark in the crush style: per-letter
// half-block letterforms joined horizontally, flanked by diagonal ╱
// fields, with a meta row (brand name + version) above and a deep→light
// brand gradient applied per row. Mirrors the composition of crush's
// internal/ui/logo but with our own DeepSeek Ocean glyphs and colors.
package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

const logoDiag = "╱"

// letterforms maps an uppercase rune to its 3-row half-block glyph.
// Every glyph is exactly 3 rows; rows within a glyph share one width.
var letterforms = map[rune][]string{
	'D': {
		"█▀▀▄",
		"█  █",
		"▀▀▀ ",
	},
	'E': {
		"█▀▀▀",
		"█▀▀ ",
		"▀▀▀▀",
	},
	'P': {
		"█▀▀▄",
		"█▀▀▀",
		"▀   ",
	},
	'S': {
		"▄▀▀▀",
		"▀▀▀▄",
		"▀▀▀▀",
	},
	'K': {
		"█ ▄▀",
		"█▀▄ ",
		"▀  ▀",
	},
	'C': {
		"▄▀▀▀",
		"█   ",
		"▀▀▀▀",
	},
	'O': {
		"▄▀▀▄",
		"█  █",
		"▀▀▀▀",
	},
}

// wordmarkRows renders word into 3 plain (uncolored) rows of letterforms,
// joined left-to-right with a single blank column between letters.
// Unknown runes are skipped.
func wordmarkRows(word string) [3]string {
	var rows [3]strings.Builder
	first := true
	for _, r := range strings.ToUpper(word) {
		glyph, ok := letterforms[r]
		if !ok {
			continue
		}
		if !first {
			for i := 0; i < 3; i++ {
				rows[i].WriteByte(' ')
			}
		}
		first = false
		for i := 0; i < 3; i++ {
			rows[i].WriteString(glyph[i])
		}
	}
	return [3]string{rows[0].String(), rows[1].String(), rows[2].String()}
}

// renderLogo composes the wide-mode banner: the gradient wordmark, flanked by
// diagonal fields. The persistent header owns brand + version, so the welcome
// logo does not repeat them.
// width is the terminal width; the caller guarantees it is wide enough
// (narrow terminals use the single-line fallback in welcome.go).
func renderLogo(t Theme, hero, brand, version string, width int) string {
	_ = brand
	_ = version
	rows := wordmarkRows(hero)
	heroW := lipgloss.Width(rows[0])

	field := lipgloss.NewStyle().Foreground(logoFieldColor(t))

	// Center stack: 3 gradient wordmark rows.
	center := make([]string, 3)
	for i := 0; i < 3; i++ {
		center[i] = ApplyForegroundGrad(lipgloss.NewStyle(), rows[i], t.BrandDeep, t.BrandLight)
	}

	const leftW = 6
	const gapW = 1 // single space on each side of the center stack
	rightW := width - leftW - heroW - 2*gapW
	if rightW < 0 {
		rightW = 0
	}

	// Every center row is exactly heroW wide (the gradient preserves width), so
	// no per-row padding is needed before the right field.
	var b strings.Builder
	for i := 0; i < 3; i++ {
		// Right field steps down one cell per row for a slanted edge.
		rw := rightW - i
		if rw < 0 {
			rw = 0
		}
		b.WriteString(field.Render(strings.Repeat(logoDiag, leftW)))
		b.WriteByte(' ')
		b.WriteString(center[i])
		b.WriteByte(' ')
		b.WriteString(field.Render(strings.Repeat(logoDiag, rw)))
		b.WriteByte('\n')
	}
	return b.String()
}

// renderHeaderBar returns a single-row header: brand rendered with the theme
// gradient, left-aligned, version (if non-empty) right-aligned faint, clamped
// to exactly `width` columns and exactly 1 line tall (never wraps).
func renderHeaderBar(t Theme, brand, version string, width int) string {
	if width <= 0 {
		return ""
	}
	brandRendered := ApplyForegroundGrad(lipgloss.NewStyle(), brand, t.BrandDeep, t.BrandLight)
	brandW := lipgloss.Width(brandRendered)

	if version == "" {
		// Pad to exact width so the row is always `width` columns.
		return lipgloss.NewStyle().Width(width).Render(clipToWidth(brandRendered, width))
	}

	verRendered := t.Hint.Render(version)
	verW := lipgloss.Width(verRendered)

	// Gap between brand and version: at least 1 space.
	gap := width - brandW - verW
	if gap < 1 {
		// Not enough room for both: truncate brand, drop version, pad to width.
		return lipgloss.NewStyle().Width(width).Render(clipToWidth(brandRendered, width))
	}

	row := brandRendered + strings.Repeat(" ", gap) + verRendered
	return clipToWidth(row, width)
}

// logoFieldColor returns the color of the diagonal ╱ field flanking the
// wordmark. It rides the border token so the field reads as a quiet structural
// frame that harmonizes with the painted canvas (border sits just above the
// bgRaised tier) instead of competing with the gradient wordmark.
func logoFieldColor(t Theme) color.Color { return t.BorderColor }
