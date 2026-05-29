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
// For the dark renderer, when fills are enabled (owned canvas + truecolor), we
// repaint the fenced code-block background to the bgWell surface token so code
// blocks read as the same recessed inset as the diff/tool panels rather than
// glamour's stock near-black. When fills are disabled (transparent mode or a
// non-truecolor terminal) we instead CLEAR the code-block background entirely —
// honoring ADR-0002's no-opaque-fills degrade contract so nothing paints a band
// over the user's terminal. The light renderer is left GitHub-ish (untouched).
//
// Both bg writes operate on copies only: `s` is a value copy of glamour's
// shared StyleConfig, but s.CodeBlock.Chroma is a *Chroma that still aliases
// the process-wide styles.DarkStyleConfig — so we copy it before mutating,
// never writing through the shared global.
func cleanStyle(name string, fills bool) ansi.StyleConfig {
	var s ansi.StyleConfig
	if name == "light" {
		s = styles.LightStyleConfig
	} else {
		s = styles.DarkStyleConfig
		var bg *string // nil clears the background (degraded); &well paints it
		if fills {
			// Read the surface token (never an inline hex) and convert to the
			// hex string glamour's StylePrimitive expects.
			well := tokenHex(DarkTheme().BgWell)
			bg = &well
		}
		s.CodeBlock.BackgroundColor = bg
		if s.CodeBlock.Chroma != nil {
			ch := *s.CodeBlock.Chroma // copy: never mutate glamour's shared global
			ch.Background.BackgroundColor = bg
			s.CodeBlock.Chroma = &ch
		}

		// Inline code (`path/like/this`). Glamour's stock dark style is red
		// (203) on a flat grey (236) with a space of padding each side — chunky
		// chips that carpet a markdown table of filenames and clash hard with
		// the ocean canvas (this was the "not beautiful" complaint). Retune to a
		// calm cyan accent with NO opaque background and no padding, so inline
		// code reads as a quiet color shift, not a block. Degrades cleanly since
		// it never paints a fill.
		codeFg := tokenHex(DarkTheme().AccentFlash)
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
func renderMarkdown(text, style string, fills bool, width int) string {
	if width <= 0 {
		return text
	}
	r := getRenderer(style, fills, width)
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

func getRenderer(style string, fills bool, width int) *glamour.TermRenderer {
	key := rendererKey{style: style, fills: fills, width: width}
	mdMu.Lock()
	defer mdMu.Unlock()
	if r, ok := mdCache[key]; ok {
		return r
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(cleanStyle(style, fills)),
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
