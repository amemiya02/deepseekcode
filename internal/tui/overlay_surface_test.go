package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// TestOverlayFillsScreenNoBlackGaps pins the redesign that killed the "navy slab
// floating on a black void": every full-screen overlay must render EXACTLY
// a.height rows, and every row must span the full width. That guarantees the
// painted bgBase surface reaches edge to edge — no unpainted (black) gap below
// the content, no ragged black to the right of a short row — and that the View
// never overflows the alt-screen. This is the exact regression the screenshots
// reported (a bgRaised panel surrounded by raw terminal black).
func TestOverlayFillsScreenNoBlackGaps(t *testing.T) {
	const w, h = 100, 40
	open := map[string]func(*App){
		"models":  func(a *App) { a.handleSlash("/models") },
		"palette": func(a *App) { a.openPalette() },
		"help":    func(a *App) { a.openHelp() },
	}
	for name, openFn := range open {
		t.Run(name, func(t *testing.T) {
			a := sizeApp(t, newKeyflowApp(t), w, h)
			openFn(a)
			if !a.overlay.IsOpen() {
				t.Fatalf("%s overlay should be open", name)
			}
			lines := strings.Split(a.renderOverlay(), "\n")
			if len(lines) != h {
				t.Fatalf("%s overlay rendered %d rows, want exactly %d — a short fill leaks black below, an overflow scrolls the alt-screen", name, len(lines), h)
			}
			for i, ln := range lines {
				if got := lipgloss.Width(ln); got != w {
					t.Fatalf("%s overlay row %d spans %d cells, want %d — a short row leaves raw black to the right", name, i, got, w)
				}
			}
		})
	}
}

// TestOverlayFillsScreenAcrossSizes runs the same edge-to-edge invariant across
// a range of terminal sizes, including the short heights where the old
// newline-padding math under-filled and left a black strip at the bottom.
func TestOverlayFillsScreenAcrossSizes(t *testing.T) {
	sizes := [][2]int{{80, 24}, {120, 50}, {100, 14}, {200, 60}}
	for _, sz := range sizes {
		w, h := sz[0], sz[1]
		a := sizeApp(t, newKeyflowApp(t), w, h)
		a.openPalette()
		lines := strings.Split(a.renderOverlay(), "\n")
		if len(lines) != h {
			t.Fatalf("palette at %dx%d rendered %d rows, want %d", w, h, len(lines), h)
		}
		for i, ln := range lines {
			if got := lipgloss.Width(ln); got != w {
				t.Fatalf("palette at %dx%d: row %d spans %d cells, want %d", w, h, i, got, w)
			}
		}
	}
}
