package tui

import (
	"strings"
	"testing"

	"charm.land/glamour/v2/styles"
)

// TestCleanStyleCodeBlockFillGate pins ADR-0002's degrade contract for the dark
// markdown renderer: the fenced code-block background is painted (bgWell) ONLY
// when fills are enabled (owned canvas + truecolor); in transparent or
// non-truecolor mode it is cleared so nothing paints an opaque band over the
// user's terminal.
func TestCleanStyleCodeBlockFillGate(t *testing.T) {
	if cleanStyle(DarkTheme(), true).CodeBlock.BackgroundColor == nil {
		t.Error("fills enabled: dark code-block background must be painted, got nil")
	}
	if bg := cleanStyle(DarkTheme(), false).CodeBlock.BackgroundColor; bg != nil {
		t.Errorf("fills disabled: dark code-block background must be cleared (no opaque fill), got %q", *bg)
	}
	// The chroma sub-style must track the same gate so highlighted code doesn't
	// keep an opaque band when the plain background is cleared.
	if ch := cleanStyle(DarkTheme(), false).CodeBlock.Chroma; ch != nil && ch.Background.BackgroundColor != nil {
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
		s := cleanStyle(DarkTheme(), fills)
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
	_ = cleanStyle(DarkTheme(), true)  // would write &well through the alias if buggy
	_ = cleanStyle(DarkTheme(), false) // would write nil through the alias if buggy
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
	on := cleanStyle(LightTheme(), true).CodeBlock.BackgroundColor
	off := cleanStyle(LightTheme(), false).CodeBlock.BackgroundColor
	switch {
	case on == nil && off == nil:
	case on != nil && off != nil && *on == *off:
	default:
		t.Errorf("light code-block bg must be identical regardless of fills: on=%v off=%v", on, off)
	}
}

// TestMarkdownInlineCodeTracksTheme verifies that inline-code color follows the
// theme's AccentFlash token — different themes produce different colors.
func TestMarkdownInlineCodeTracksTheme(t *testing.T) {
	midnight := cleanStyle(MidnightTheme(), true)
	aurora := cleanStyle(AuroraTheme(), true)

	midnightColor := tokenHex(MidnightTheme().AccentFlash)
	auroraColor := tokenHex(AuroraTheme().AccentFlash)

	if midnight.Code.Color == nil {
		t.Fatal("Midnight inline code color is nil")
	}
	if *midnight.Code.Color != midnightColor {
		t.Errorf("Midnight inline code: got %q, want %q", *midnight.Code.Color, midnightColor)
	}
	if aurora.Code.Color == nil {
		t.Fatal("Aurora inline code color is nil")
	}
	if *aurora.Code.Color != auroraColor {
		t.Errorf("Aurora inline code: got %q, want %q", *aurora.Code.Color, auroraColor)
	}
	if midnightColor == auroraColor {
		t.Error("Midnight and Aurora inline code colors should differ")
	}
}

// TestMarkdownCodeBlockBgTracksTheme verifies that code-block background
// follows the theme's BgWell token when fills are on, and is nil when off.
func TestMarkdownCodeBlockBgTracksTheme(t *testing.T) {
	nebula := NebulaTheme()
	s := cleanStyle(nebula, true)
	want := tokenHex(nebula.BgWell)
	if s.CodeBlock.BackgroundColor == nil {
		t.Fatal("fills=true: Nebula code-block bg must be painted")
	}
	if *s.CodeBlock.BackgroundColor != want {
		t.Errorf("Nebula code-block bg: got %q, want %q", *s.CodeBlock.BackgroundColor, want)
	}

	s2 := cleanStyle(nebula, false)
	if s2.CodeBlock.BackgroundColor != nil {
		t.Errorf("fills=false: Nebula code-block bg must be nil, got %q", *s2.CodeBlock.BackgroundColor)
	}
}

// TestMarkdownLightUsesLightBase verifies that the light theme uses a different
// glamour base than the dark theme.
func TestMarkdownLightUsesLightBase(t *testing.T) {
	light := cleanStyle(LightTheme(), true)
	dark := cleanStyle(DarkTheme(), true)
	// Compare a field that differs between light and dark glamour bases:
	// the Document style's color.
	lightColor := light.Document.StylePrimitive.Color
	darkColor := dark.Document.StylePrimitive.Color
	if lightColor == nil && darkColor == nil {
		t.Skip("both nil — cannot distinguish")
	}
	if lightColor != nil && darkColor != nil && *lightColor == *darkColor {
		t.Error("light and dark Document color should differ")
	}
}

// TestRenderMarkdownAuroraSmoke verifies renderMarkdown doesn't panic with a
// non-default theme.
func TestRenderMarkdownAuroraSmoke(t *testing.T) {
	out := renderMarkdown("# hi\n`code`", AuroraTheme(), true, 80)
	if out == "" {
		t.Error("expected non-empty output")
	}
	if strings.Contains(out, "# ") {
		t.Error("heading prefix should be stripped")
	}
}
