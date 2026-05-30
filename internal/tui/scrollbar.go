// scrollbar.go overlays a one-cell vertical scrollbar onto the right edge of
// the rendered viewport body. The thumb is brand-light and the track rides the
// border token so the bar is clearly visible against the painted canvas while
// still reading as chrome rather than content. The thumb/track glyphs are the
// shared ScrollbarThumb / ScrollbarTrack symbols.
package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	reflowtruncate "github.com/muesli/reflow/truncate"
)

// overlayScrollbar replaces the rightmost cell of each viewport row with a
// scrollbar glyph. It is a no-op when the whole scrollback fits on screen
// (nothing to scroll) so the bar only appears when it carries information.
//
// The thumb spans a proportional slice of the track sized to the visible /
// total ratio and positioned by the current scroll offset; both endpoints are
// clamped so the thumb stays at least one cell tall and never overruns the
// track.
func (a *App) overlayScrollbar(view string) string {
	lines := strings.Split(view, "\n")
	h := len(lines)
	if h == 0 || a.width <= 0 {
		return view
	}

	total := a.vp.TotalLineCount()
	if total <= h {
		// Everything fits; no scrollbar — drawing one would only add noise.
		return view
	}

	thumbStart, thumbEnd := scrollbarThumb(h, total, a.vp.ScrollPercent())

	thumbStyle := lipgloss.NewStyle().Foreground(a.theme.BrandLight)
	trackStyle := lipgloss.NewStyle().Foreground(a.theme.BorderColor)

	for i := 0; i < h; i++ {
		glyph := trackStyle.Render(ScrollbarTrack)
		if i >= thumbStart && i < thumbEnd {
			glyph = thumbStyle.Render(ScrollbarThumb)
		}
		lines[i] = replaceLastCell(lines[i], glyph, a.width)
	}
	return strings.Join(lines, "\n")
}

// scrollbarThumb returns the [start, end) row range of the thumb within a
// track of trackH rows, given the total line count and the scroll fraction
// (0..1). The thumb height is proportional to the visible window and clamped
// to at least one row; its position is interpolated across the free travel.
func scrollbarThumb(trackH, total int, pct float64) (int, int) {
	if trackH <= 0 || total <= 0 {
		return 0, 0
	}
	thumbH := trackH * trackH / total
	if thumbH < 1 {
		thumbH = 1
	}
	if thumbH > trackH {
		thumbH = trackH
	}
	travel := trackH - thumbH
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	start := int(pct*float64(travel) + 0.5)
	if start < 0 {
		start = 0
	}
	if start > travel {
		start = travel
	}
	return start, start + thumbH
}

// replaceLastCell swaps the rightmost display cell of a rendered line for the
// given (already-styled) glyph, padding short lines out to width-1 first so the
// bar always sits flush against the right edge. The incoming line carries ANSI
// styling, so we operate on display width via lipgloss and rebuild by trimming
// one printable cell off the right.
func replaceLastCell(line, glyph string, width int) string {
	w := lipgloss.Width(line)
	if w < width {
		// Pad up to width-1 so the glyph lands in the last column.
		line += strings.Repeat(" ", width-1-w)
		return line + glyph
	}
	// Line already fills the row: drop its final printable cell, then append
	// the glyph. reflow/truncate is ANSI-aware so styling on the kept prefix
	// survives the trim (it also closes any open SGR at the cut).
	trimmed := reflowtruncate.String(line, uint(width-1))
	return trimmed + glyph
}
