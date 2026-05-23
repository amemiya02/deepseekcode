// markdown.go renders assistant text through Glamour (markdown-aware,
// ANSI-styled) and reasoning / tool output through word-aware wrap.
//
// A single Glamour renderer is cached per (style, width) tuple — the
// renderer constructor parses a non-trivial style JSON, so reusing
// across refreshes keeps the per-keystroke render cost negligible.
package tui

import (
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/muesli/reflow/wordwrap"
)

type rendererKey struct {
	style string // "dark" | "light"
	width int
}

var (
	mdMu    sync.Mutex
	mdCache = map[rendererKey]*glamour.TermRenderer{}
)

// renderMarkdown turns markdown text into ANSI-styled output that fits
// the given column width. Falls back to the raw text on any error so a
// markdown bug never blanks the UI.
func renderMarkdown(text, style string, width int) string {
	if width <= 0 {
		return text
	}
	r := getRenderer(style, width)
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

func getRenderer(style string, width int) *glamour.TermRenderer {
	key := rendererKey{style: style, width: width}
	mdMu.Lock()
	defer mdMu.Unlock()
	if r, ok := mdCache[key]; ok {
		return r
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
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
