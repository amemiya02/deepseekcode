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

// TestStreamingMarkdownCachesSafePrefix verifies that streamingMarkdown
// caches the render of the largest safe prefix (ending at a "\n\n" outside
// fenced code blocks) and reuses it on subsequent calls.
func TestStreamingMarkdownCachesSafePrefix(t *testing.T) {
	var m streamingMarkdown
	th := DarkTheme()
	w := 80

	// First call: "Intro paragraph.\n\nSecond par" — boundary at "\n\n" caches prefix.
	text1 := "Intro paragraph.\n\nSecond par"
	out1 := m.render(text1, th, true, w)
	if out1 == "" {
		t.Fatal("expected non-empty output for text1")
	}
	if m.stablePrefix != "Intro paragraph.\n\n" {
		t.Errorf("stablePrefix = %q, want %q", m.stablePrefix, "Intro paragraph.\n\n")
	}

	// Second call: text extends the tail. Prefix should be reused.
	text2 := "Intro paragraph.\n\nSecond paragraph done."
	out2 := m.render(text2, th, true, w)
	if out2 == "" {
		t.Fatal("expected non-empty output for text2")
	}
	if m.stablePrefix != "Intro paragraph.\n\n" {
		t.Errorf("stablePrefix changed unexpectedly: %q", m.stablePrefix)
	}
	if m.hits != 1 {
		t.Errorf("expected 1 cache hit after second render, got %d", m.hits)
	}

	// The finalized output should have the full text (strip ANSI for check).
	stripped := stripANSI(out2)
	if !strings.Contains(stripped, "Second paragraph done.") {
		t.Errorf("output missing tail text: %q", stripped)
	}
}

// TestStreamingMarkdownFinalizedMatchesRenderMarkdown verifies the seam:
// a fully-finalized text rendered through streamingMarkdown produces output
// whose visible text equals renderMarkdown of the same text.
func TestStreamingMarkdownFinalizedMatchesRenderMarkdown(t *testing.T) {
	var m streamingMarkdown
	th := DarkTheme()
	w := 80
	text := "First paragraph.\n\nSecond paragraph."

	streamOut := m.render(text, th, true, w)
	directOut := renderMarkdown(text, th, true, w)

	// Both should contain the same visible content.
	if stripANSI(streamOut) != stripANSI(directOut) {
		t.Errorf("streaming vs direct render mismatch:\nstream=%q\ndirect=%q", stripANSI(streamOut), stripANSI(directOut))
	}
}

// TestStreamingMarkdownReuseMatchesRenderMarkdown verifies the critical seam:
// after the cache is populated, a longer string rendered through the reuse
// path contains the same visible content as renderMarkdown of the same text.
func TestStreamingMarkdownReuseMatchesRenderMarkdown(t *testing.T) {
	var m streamingMarkdown
	th := DarkTheme()
	w := 80
	// First call populates the cache.
	_ = m.render("Intro paragraph.\n\nSecond par", th, true, w)
	// Second call exercises the reuse path.
	got := stripANSI(m.render("Intro paragraph.\n\nSecond paragraph done.", th, true, w))
	want := stripANSI(renderMarkdown("Intro paragraph.\n\nSecond paragraph done.", th, true, w))
	// Both must contain the key content words.
	for _, word := range []string{"Intro", "paragraph.", "Second", "done."} {
		if !strings.Contains(got, word) {
			t.Errorf("reuse output missing %q: %q", word, got)
		}
		if !strings.Contains(want, word) {
			t.Errorf("full output missing %q: %q", word, want)
		}
	}
	// Both must have the same number of non-blank content lines.
	gotLines := nonBlankLines(got)
	wantLines := nonBlankLines(want)
	if len(gotLines) != len(wantLines) {
		t.Errorf("line count mismatch: got %d, want %d\ngot=%q\nwant=%q", len(gotLines), len(wantLines), got, want)
	}
	if m.hits != 1 {
		t.Errorf("expected 1 cache hit, got %d", m.hits)
	}
}

// TestStreamingMarkdownDoesNotRerenderExtendedPrefix proves that when the
// new text introduces a later safe boundary, the streaming reuse path does
// NOT re-render the expanded prefix — it renders only the tail.
func TestStreamingMarkdownDoesNotRerenderExtendedPrefix(t *testing.T) {
	orig := renderMarkdownFn
	var calls []string
	renderMarkdownFn = func(text string, th Theme, fills bool, width int) string {
		calls = append(calls, text)
		return orig(text, th, fills, width)
	}
	defer func() { renderMarkdownFn = orig }()

	var m streamingMarkdown
	th := DarkTheme()
	_ = m.render("Intro paragraph.\n\nSecond par", th, true, 80)
	calls = nil
	_ = m.render("Intro paragraph.\n\nSecond paragraph done.\n\nThird par", th, true, 80)

	if len(calls) != 1 {
		t.Fatalf("reuse with new safe boundary should render only one tail; got %d calls: %#v", len(calls), calls)
	}
}

func nonBlankLines(s string) []string {
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

// TestStreamingMarkdownReuseRendersOnlyTail proves the reuse path does NOT
// call the markdown renderer for the full text — only for the changed tail.
func TestStreamingMarkdownReuseRendersOnlyTail(t *testing.T) {
	// Swap in a counting mock.
	orig := renderMarkdownFn
	var calls []string
	renderMarkdownFn = func(text string, th Theme, fills bool, width int) string {
		calls = append(calls, text)
		return orig(text, th, fills, width)
	}
	defer func() { renderMarkdownFn = orig }()

	var m streamingMarkdown
	th := DarkTheme()
	w := 80

	// First call: full render + prefix cache.
	calls = nil
	_ = m.render("Intro paragraph.\n\nSecond par", th, true, w)
	if len(calls) != 2 {
		t.Fatalf("first call: expected 2 renders (full + prefix), got %d: %v", len(calls), calls)
	}

	// Second call: should render ONLY the tail, not the full text.
	calls = nil
	_ = m.render("Intro paragraph.\n\nSecond paragraph done.", th, true, w)
	if len(calls) != 1 {
		t.Fatalf("reuse call: expected 1 render (tail only), got %d: %v", len(calls), calls)
	}
	if calls[0] != "Second paragraph done." {
		t.Errorf("reuse call rendered wrong text: %q", calls[0])
	}
}

// TestStreamingMarkdownEdgeCases ensures no panic on edge-case inputs.
func TestStreamingMarkdownEdgeCases(t *testing.T) {
	var m streamingMarkdown
	th := DarkTheme()

	// Empty text.
	if got := m.render("", th, true, 80); got != "" {
		t.Errorf("empty text: got %q, want %q", got, "")
	}

	// No blank lines — should fall back to full render.
	m.reset()
	got := m.render("no blank lines here", th, true, 80)
	if got == "" {
		t.Error("no-blank-lines: expected non-empty output")
	}

	// Unterminated fenced code block — boundary must be before the open fence.
	m.reset()
	text := "before\n\n```go\nunterminated code"
	out := m.render(text, th, true, 80)
	if out == "" {
		t.Error("unterminated fence: expected non-empty output")
	}

	// Width 0 — should return raw text.
	m.reset()
	if got := m.render("some text", th, true, 0); got != "some text" {
		t.Errorf("width 0: got %q, want %q", got, "some text")
	}
}

// TestStreamingMarkdownResetClearsCache verifies reset() clears all cached state.
func TestStreamingMarkdownResetClearsCache(t *testing.T) {
	var m streamingMarkdown
	th := DarkTheme()
	w := 80

	m.render("hello\n\nworld", th, true, w)
	if m.stablePrefix == "" {
		t.Fatal("expected cached prefix after render")
	}

	m.reset()
	if m.stablePrefix != "" {
		t.Errorf("stablePrefix not cleared: %q", m.stablePrefix)
	}
	if m.renderedPrefix != "" {
		t.Errorf("renderedPrefix not cleared: %q", m.renderedPrefix)
	}
	if m.hits != 0 {
		t.Errorf("hits not reset: %d", m.hits)
	}
}

// TestStreamingMarkdownKeyChangeInvalidatesCache verifies that a theme/width
// change forces a full re-render.
func TestStreamingMarkdownKeyChangeInvalidatesCache(t *testing.T) {
	var m streamingMarkdown
	th := DarkTheme()
	w := 80

	m.render("hello\n\nworld", th, true, w)
	if m.stablePrefix == "" {
		t.Fatal("expected cached prefix")
	}

	// Change width — should invalidate.
	out := m.render("hello\n\nworld", th, true, 40)
	if out == "" {
		t.Error("width change: expected non-empty output")
	}
	// Cache should be repopulated with new key.
	if m.key.width != 40 {
		t.Errorf("key width not updated: %d", m.key.width)
	}
}

// TestSafeBoundaryFindsLastEvenFence verifies the safe boundary logic.
func TestSafeBoundaryFindsLastEvenFence(t *testing.T) {
	tests := []struct {
		text string
		want int
	}{
		{"", 0},
		{"no blank lines", 0},
		{"hello\n\nworld", len("hello\n\n")},
		{"a\n\nb\n\nc", len("a\n\nb\n\n")},
		{"before\n\n```go\ncode\n```\n\nafter", len("before\n\n```go\ncode\n```\n\n")},
	}
	for _, tt := range tests {
		got := safeBoundary(tt.text)
		if got != tt.want {
			t.Errorf("safeBoundary(%q) = %d, want %d", tt.text, got, tt.want)
		}
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
