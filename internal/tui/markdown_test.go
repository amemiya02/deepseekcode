package tui

import (
	"testing"

	"charm.land/glamour/v2/styles"
)

// TestCleanStyleCodeBlockFillGate pins ADR-0002's degrade contract for the dark
// markdown renderer: the fenced code-block background is painted (bgWell) ONLY
// when fills are enabled (owned canvas + truecolor); in transparent or
// non-truecolor mode it is cleared so nothing paints an opaque band over the
// user's terminal.
func TestCleanStyleCodeBlockFillGate(t *testing.T) {
	if cleanStyle("dark", true).CodeBlock.BackgroundColor == nil {
		t.Error("fills enabled: dark code-block background must be painted, got nil")
	}
	if bg := cleanStyle("dark", false).CodeBlock.BackgroundColor; bg != nil {
		t.Errorf("fills disabled: dark code-block background must be cleared (no opaque fill), got %q", *bg)
	}
	// The chroma sub-style must track the same gate so highlighted code doesn't
	// keep an opaque band when the plain background is cleared.
	if ch := cleanStyle("dark", false).CodeBlock.Chroma; ch != nil && ch.Background.BackgroundColor != nil {
		t.Errorf("fills disabled: chroma code-block background must be cleared, got %q", *ch.Background.BackgroundColor)
	}
}

// TestCleanStyleInlineCodeRetuned pins the inline-code retune: glamour's stock
// red-on-grey padded chip is replaced by a calm cyan accent with NO opaque
// background and no padding, so a markdown table full of `path/like/this`
// doesn't carpet the ocean canvas with grey blocks. Holds in both fill modes
// (inline code never paints a fill, so it degrades trivially).
func TestCleanStyleInlineCodeRetuned(t *testing.T) {
	for _, fills := range []bool{true, false} {
		s := cleanStyle("dark", fills)
		if s.Code.BackgroundColor != nil {
			t.Errorf("fills=%v: inline code must have no opaque background, got %q", fills, *s.Code.BackgroundColor)
		}
		if s.Code.Color == nil {
			t.Errorf("fills=%v: inline code must set a calm accent color (not glamour's red)", fills)
		}
		if s.Code.Prefix != "" || s.Code.Suffix != "" {
			t.Errorf("fills=%v: inline code padding must be removed, got prefix=%q suffix=%q", fills, s.Code.Prefix, s.Code.Suffix)
		}
	}
}

// TestCleanStyleDoesNotMutateGlamourGlobal guards the aliasing hazard: cleanStyle
// copies glamour's shared StyleConfig by value, but CodeBlock.Chroma is a
// *Chroma that aliases the process-global styles.DarkStyleConfig. Writing
// through it would silently corrupt every future renderer (and race off the
// mdMu mutex). We must copy the Chroma before mutating — pin that the global is
// never touched.
func TestCleanStyleDoesNotMutateGlamourGlobal(t *testing.T) {
	var before *string
	if g := styles.DarkStyleConfig.CodeBlock.Chroma; g != nil {
		before = g.Background.BackgroundColor
	}
	_ = cleanStyle("dark", true)  // would write &well through the alias if buggy
	_ = cleanStyle("dark", false) // would write nil through the alias if buggy
	var after *string
	if g := styles.DarkStyleConfig.CodeBlock.Chroma; g != nil {
		after = g.Background.BackgroundColor
	}
	if before != after {
		t.Errorf("cleanStyle mutated glamour's global Chroma background pointer: before=%v after=%v", before, after)
	}
}

// TestCleanStyleLightUnaffectedByFills documents the intentional design choice
// that the light renderer is left GitHub-ish (untouched) regardless of the
// fills gate — so a future change to the dark gate can't silently start
// repainting light code blocks.
func TestCleanStyleLightUnaffectedByFills(t *testing.T) {
	on := cleanStyle("light", true).CodeBlock.BackgroundColor
	off := cleanStyle("light", false).CodeBlock.BackgroundColor
	switch {
	case on == nil && off == nil:
	case on != nil && off != nil && *on == *off:
	default:
		t.Errorf("light code-block bg must be identical regardless of fills: on=%v off=%v", on, off)
	}
}
