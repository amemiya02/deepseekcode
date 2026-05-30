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

// renderMarkdownFn is the markdown rendering function. It is a package-level
// variable so tests can swap in a mock to count calls or inject behavior.
// Production code uses renderMarkdown; tests may reassign this.
var renderMarkdownFn = renderMarkdown

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

// streamingMarkdown incrementally renders an in-progress markdown stream.
// It caches the Glamour render of the largest "safe" prefix of the source
// text and re-renders only the remainder per call. A safe prefix ends at
// the last "\n\n" (blank line) that is NOT inside an open fenced code
// block — i.e. the count of "```" occurrences before that boundary is
// even. Zero value is ready to use.
type streamingMarkdown struct {
	// stablePrefix is the source text of the cached safe prefix.
	stablePrefix string
	// renderedPrefix is the Glamour render of stablePrefix.
	renderedPrefix string
	// key tracks (theme, fills, width) to detect config drift.
	key streamKey
	// hits counts cache reuses (for testing).
	hits int
}

type streamKey struct {
	theme string
	fills bool
	width int
}

// render returns ANSI-styled markdown for the full text, reusing the cached
// safe-prefix render and rendering only the tail. It falls back to a full
// renderMarkdown when: the (theme,fills,width) key changed, no safe boundary
// exists, or the cache is empty.
func (m *streamingMarkdown) render(text string, t Theme, fills bool, width int) string {
	if width <= 0 {
		return text
	}
	k := streamKey{theme: t.Name, fills: fills, width: width}

	// Key mismatch or empty text: full render, reset cache.
	if text == "" || m.key != k || m.stablePrefix == "" {
		if text == "" {
			return ""
		}
		out := renderMarkdownFn(text, t, fills, width)
		// Try to cache a safe prefix for next time.
		if boundary := safeBoundary(text); boundary > 0 {
			m.stablePrefix = text[:boundary]
			m.renderedPrefix = renderMarkdownFn(m.stablePrefix, t, fills, width)
			m.key = k
		}
		return out
	}

	// The cached prefix is still a prefix of the current text: reuse the
	// cached rendered prefix and only re-render the changed tail.
	if len(text) > len(m.stablePrefix) && text[:len(m.stablePrefix)] == m.stablePrefix {
		tail := text[len(m.stablePrefix):]
		renderedTail := renderMarkdownFn(tail, t, fills, width)
		m.hits++
		// Update stablePrefix to the new boundary for the next call,
		// but do NOT re-render the expanded prefix now — that would
		// violate the "only render changed tail" contract. The cached
		// renderedPrefix stays as-is; it will be refreshed on the next
		// full render (e.g. at finalize/reset).
		if newBoundary := safeBoundary(text); newBoundary > len(m.stablePrefix) {
			m.stablePrefix = text[:newBoundary]
		}
		return stitchRenderedMarkdown(m.renderedPrefix, renderedTail)
	}

	// Prefix changed (shouldn't happen for append-only streams, but guard).
	out := renderMarkdownFn(text, t, fills, width)
	if boundary := safeBoundary(text); boundary > 0 {
		m.stablePrefix = text[:boundary]
		m.renderedPrefix = renderMarkdownFn(m.stablePrefix, t, fills, width)
		m.key = k
	}
	return out
}

// reset clears the cache (called on stream finalize).
func (m *streamingMarkdown) reset() {
	m.stablePrefix = ""
	m.renderedPrefix = ""
	m.key = streamKey{}
	m.hits = 0
}

// stitchRenderedMarkdown joins a cached rendered prefix with a freshly
// rendered tail, preserving Glamour's paragraph separator. The prefix
// was rendered from source ending at "\n\n"; renderMarkdown strips ALL
// trailing newlines, so the prefix loses the paragraph separator. The
// raw Glamour output for a source ending at "\n\n" is "...padded\n\n";
// after stripping both newlines, we need to re-add one "\n" to restore
// the paragraph break. The tail starts with Glamour's leading "\n" +
// padded text, so the result is prefix + "\n" + "\n" + padded tail —
// matching the full render's blank separator line.
func stitchRenderedMarkdown(renderedPrefix, renderedTail string) string {
	if renderedPrefix == "" {
		return renderedTail
	}
	return renderedPrefix + "\n" + renderedTail
}

// safeBoundary finds the byte offset of the end of the last "\n\n" in text
// whose preceding "```" count is even (not inside an open fenced code block).
// Returns 0 if no safe boundary exists.
func safeBoundary(text string) int {
	// Scan for "\n\n" boundaries from the end, checking fence parity.
	lastBoundary := 0
	for i := 0; i < len(text)-1; i++ {
		if text[i] == '\n' && text[i+1] == '\n' {
			prefix := text[:i]
			fenceCount := strings.Count(prefix, "```")
			if fenceCount%2 == 0 {
				lastBoundary = i + 2 // include both newlines
			}
		}
	}
	return lastBoundary
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
