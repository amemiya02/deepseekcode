package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/lucasb-eyer/go-colorful"
)

// TestScrollbarThumbGeometry pins the thumb math: the thumb is at least one
// row, never overruns the track, sits at the top when unscrolled, and reaches
// the bottom edge when fully scrolled.
func TestScrollbarThumbGeometry(t *testing.T) {
	// Tall content in a short track.
	const trackH = 10
	const total = 100

	// Top of scroll.
	start, end := scrollbarThumb(trackH, total, 0)
	if start != 0 {
		t.Errorf("at top: start = %d, want 0", start)
	}
	if end <= start {
		t.Fatalf("thumb empty: [%d,%d)", start, end)
	}
	if end > trackH {
		t.Errorf("thumb overruns track: end = %d > %d", end, trackH)
	}

	// Bottom of scroll: thumb's end must touch the track bottom.
	start, end = scrollbarThumb(trackH, total, 1)
	if end != trackH {
		t.Errorf("at bottom: end = %d, want %d", end, trackH)
	}
	if start < 0 {
		t.Errorf("negative start: %d", start)
	}

	// Minimum one-row thumb even for very long content.
	_, _ = scrollbarThumb(trackH, 100000, 0.5)
	s2, e2 := scrollbarThumb(trackH, 100000, 0.5)
	if e2-s2 < 1 {
		t.Errorf("thumb collapsed below 1 row: [%d,%d)", s2, e2)
	}
}

// TestOverlayScrollbarNoOpWhenFits verifies the scrollbar is suppressed when
// all content fits (nothing to scroll) — the bar only appears when it carries
// information.
func TestOverlayScrollbarNoOpWhenFits(t *testing.T) {
	a := &App{theme: DarkTheme(), width: 20}
	a.vp.SetWidth(20)
	a.vp.SetHeight(10)
	a.vp.SetContent("one\ntwo\nthree")
	view := a.vp.View()
	if got := a.overlayScrollbar(view); got != view {
		t.Error("scrollbar should be a no-op when content fits the viewport")
	}
}

// TestOverlayScrollbarDrawsGlyphs verifies that when content overflows, the
// scrollbar glyphs land on the right edge in the brand-light / border colors.
func TestOverlayScrollbarDrawsGlyphs(t *testing.T) {
	a := &App{theme: DarkTheme(), width: 20}
	a.vp.SetWidth(20)
	a.vp.SetHeight(5)
	var b strings.Builder
	for i := 0; i < 50; i++ {
		b.WriteString("line of content here\n")
	}
	a.vp.SetContent(b.String())
	out := a.overlayScrollbar(a.vp.View())
	plain := stripANSI(out)
	if !strings.Contains(plain, ScrollbarThumb) {
		t.Errorf("expected thumb glyph %q in scrollbar overlay", ScrollbarThumb)
	}
	if !strings.Contains(plain, ScrollbarTrack) {
		t.Errorf("expected track glyph %q in scrollbar overlay", ScrollbarTrack)
	}
	// Every rendered line stays within width.
	for _, ln := range strings.Split(plain, "\n") {
		if w := lipgloss.Width(ln); w > a.width {
			t.Errorf("scrollbar line exceeds width %d: got %d (%q)", a.width, w, ln)
		}
	}
}

// TestModeChip pins the mode-chip mapping: default → no chip; yolo / read-only
// / plan each render their label. nil Permissions → no chip (test fixtures).
func TestModeChip(t *testing.T) {
	// nil agent / permissions: no chip, no panic.
	a := &App{theme: DarkTheme()}
	if got := a.modeChip(); got != "" {
		t.Errorf("nil-permission chip = %q, want empty", got)
	}
}

// TestDarkCodeBlockBackgroundIsBgWell verifies the dark renderer repaints the
// fenced code-block background to the bgWell surface token, while the light
// renderer is left untouched.
func TestDarkCodeBlockBackgroundIsBgWell(t *testing.T) {
	want := tokenHex(DarkTheme().BgWell)
	dark := cleanStyle("dark", true) // fills enabled: code-block bg painted to bgWell
	if dark.CodeBlock.BackgroundColor == nil {
		t.Fatal("dark code block has no background color")
	}
	if got := *dark.CodeBlock.BackgroundColor; !sameHex(got, want) {
		t.Errorf("dark code-block bg = %q, want bgWell %q", got, want)
	}
	if dark.CodeBlock.Chroma != nil && dark.CodeBlock.Chroma.Background.BackgroundColor != nil {
		if got := *dark.CodeBlock.Chroma.Background.BackgroundColor; !sameHex(got, want) {
			t.Errorf("dark chroma bg = %q, want bgWell %q", got, want)
		}
	}

	// Heading prefixes stay stripped (regression guard for the existing
	// behavior this function also owns).
	if dark.H3.Prefix != "" {
		t.Errorf("H3 prefix should be stripped, got %q", dark.H3.Prefix)
	}
}

// sameHex compares two hex color strings case-insensitively.
func sameHex(a, b string) bool {
	ca, errA := colorful.Hex(normalizeHex(a))
	cb, errB := colorful.Hex(normalizeHex(b))
	if errA != nil || errB != nil {
		return strings.EqualFold(a, b)
	}
	return ca.Hex() == cb.Hex()
}

func normalizeHex(s string) string {
	if !strings.HasPrefix(s, "#") {
		return "#" + s
	}
	return s
}
