// markdown.go renders assistant text through Glamour (markdown-aware,
// ANSI-styled) and reasoning / tool output through word-aware wrap.
//
// A single Glamour renderer is cached per (style, width) tuple — the
// renderer constructor parses a non-trivial style JSON, so reusing
// across refreshes keeps the per-keystroke render cost negligible.
package tui

import (
	"image/color"
	"strings"
	"sync"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	"github.com/lucasb-eyer/go-colorful"
	"github.com/muesli/reflow/wordwrap"
)

type rendererKey struct {
	style string // "dark" | "light"
	fills bool   // whether opaque code-block backgrounds are painted
	width int
}

var (
	mdMu    sync.Mutex
	mdCache = map[rendererKey]*glamour.TermRenderer{}
)

// cleanStyle returns a Glamour style with heading prefixes / suffixes
// stripped. The shipped dark/light styles render H3 as literal "### "
// before the heading text — useful as a visual cue for plain terminals
// but visual noise in a TUI that already styles headings with bold +
// color. We want Claude-Code-style clean headings.
//
// Polarity is driven by t.IsLight(): light themes use the light glamour
// base; dark themes use the dark base with per-theme code-block bg
// (t.BgWell) and inline-code color (t.AccentFlash). When fills are
// disabled (transparent mode or non-truecolor) the code-block background
// is cleared entirely — honoring ADR-0002's no-opaque-fills degrade.
//
// Both bg writes operate on copies only: `s` is a value copy of glamour's
// shared StyleConfig, but s.CodeBlock.Chroma is a *Chroma that still aliases
// the process-wide styles.DarkStyleConfig — so we copy it before mutating,
// never writing through the shared global.
func cleanStyle(t Theme, fills bool) ansi.StyleConfig {
	var s ansi.StyleConfig
	if t.IsLight() {
		s = styles.LightStyleConfig
	} else {
		s = styles.DarkStyleConfig
		var bg *string // nil clears the background (degraded); &well paints it
		if fills {
			// Read the surface token (never an inline hex) and convert to the
			// hex string glamour's StylePrimitive expects.
			well := tokenHex(t.BgWell)
			bg = &well
		}
		s.CodeBlock.BackgroundColor = bg
		if s.CodeBlock.Chroma != nil {
			ch := *s.CodeBlock.Chroma // copy: never mutate glamour's shared global
			ch.Background.BackgroundColor = bg
			s.CodeBlock.Chroma = &ch
		}

		// Inline code (`path/like/this`). Retune to the theme's accent flash
		// with NO opaque background and no padding, so inline code reads as a
		// quiet color shift, not a block. Degrades cleanly since it never
		// paints a fill.
		codeFg := tokenHex(t.AccentFlash)
		s.Code.Color = &codeFg
		s.Code.BackgroundColor = nil
		s.Code.Prefix = ""
		s.Code.Suffix = ""
	}
	for _, h := range []*ansi.StyleBlock{&s.H1, &s.H2, &s.H3, &s.H4, &s.H5, &s.H6} {
		h.Prefix = ""
		h.Suffix = ""
	}
	return s
}

// tokenHex renders a theme token color as the "#rrggbb" string glamour wants.
// It lets call sites pass a semantic token (e.g. theme.BgWell) instead of
// re-typing a hex literal.
func tokenHex(c color.Color) string {
	cf, _ := colorful.MakeColor(c)
	return cf.Hex()
}

// renderMarkdown turns markdown text into ANSI-styled output that fits
// the given column width. Falls back to the raw text on any error so a
// markdown bug never blanks the UI.
func renderMarkdown(text string, t Theme, fills bool, width int) string {
	if width <= 0 {
		return text
	}
	r := getRenderer(t, fills, width)
	if r == nil {
		return text
	}
	out, err := r.Render(text)
	if err != nil {
		return text
	}
	// Glamour appends a trailing newline; caller adds its own.
	return strings.TrimRight(out, "\n")
}

func getRenderer(t Theme, fills bool, width int) *glamour.TermRenderer {
	key := rendererKey{style: t.Name, fills: fills, width: width}
	mdMu.Lock()
	defer mdMu.Unlock()
	if r, ok := mdCache[key]; ok {
		return r
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(cleanStyle(t, fills)),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil
	}
	mdCache[key] = r
	return r
}

// wrapWords wraps plain text at the given column, breaking on
// whitespace and respecting multi-byte runes. Used for reasoning text
// (not markdown) where Glamour would be overkill.
func wrapWords(text string, width int) string {
	if width <= 0 {
		return text
	}
	return wordwrap.String(text, width)
}
