package tui

import (
	"image/color"
	"io"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// Highlight uses chroma to syntax-highlight source by lang, returning
// an ANSI-colored string. Background is locked to the Ocean card body
// color. Returns source unchanged if lang is empty or unrecognized.
func Highlight(t Theme, source, lang string) string {
	if source == "" {
		return source
	}
	lexer := lexers.Get(lang)
	if lexer == nil {
		lexer = lexers.Analyse(source)
	}
	if lexer == nil {
		return source
	}
	lexer = chroma.Coalesce(lexer)

	// Pick a chroma style that matches the theme's background: dracula's dark
	// palette is illegible on a light terminal, so the light theme gets a
	// light-background style. styles.Get falls back internally on an unknown
	// name; we also guard nil explicitly.
	styleName := "dracula"
	if t.Name == "light" {
		styleName = "github"
	}
	style := styles.Get(styleName)
	if style == nil {
		style = styles.Fallback
	}

	formatter := chromaFormatter(t.CardBody.GetBackground())
	iter, err := lexer.Tokenise(nil, source)
	if err != nil {
		return source
	}
	var buf strings.Builder
	if err := formatter.Format(&buf, style, iter); err != nil {
		return source
	}
	return buf.String()
}

// highlightOnBg syntax-highlights source by lang exactly like Highlight, but
// FORCES every token's background to bg instead of CardBody. It is the entry
// point used by the unified diff view so highlighted code sits on the filled
// +/- band (or the recessed well for context lines). Returns source unchanged
// if lang is empty/unrecognized or lexing fails.
func highlightOnBg(t Theme, source, lang string, bg color.Color) string {
	if source == "" {
		return source
	}
	lexer := lexers.Get(lang)
	if lexer == nil {
		lexer = lexers.Analyse(source)
	}
	if lexer == nil {
		return source
	}
	lexer = chroma.Coalesce(lexer)

	styleName := "dracula"
	if t.Name == "light" {
		styleName = "github"
	}
	style := styles.Get(styleName)
	if style == nil {
		style = styles.Fallback
	}

	formatter := chromaFormatter(bg)
	iter, err := lexer.Tokenise(nil, source)
	if err != nil {
		return source
	}
	var buf strings.Builder
	if err := formatter.Format(&buf, style, iter); err != nil {
		return source
	}
	return buf.String()
}

// chromaFormatter returns a chroma.Formatter that maps token types to
// lipgloss foreground styles with a fixed background color.
func chromaFormatter(bg color.Color) chroma.Formatter {
	return chroma.FormatterFunc(func(w io.Writer, style *chroma.Style, iter chroma.Iterator) error {
		for tok := iter(); tok != chroma.EOF; tok = iter() {
			entry := style.Get(tok.Type)
			fg := entry.Colour
			s := lipgloss.NewStyle()
			if fg != chroma.Colour(0) {
				s = s.Foreground(lipgloss.Color(fg.String()))
			}
			if bg != nil {
				s = s.Background(bg)
			}
			if entry.Bold == chroma.Yes {
				s = s.Bold(true)
			}
			if entry.Italic == chroma.Yes {
				s = s.Italic(true)
			}
			if entry.Underline == chroma.Yes {
				s = s.Underline(true)
			}
			// Render each line of the token separately. lipgloss block-pads
			// multi-line input to the widest line, and chroma emits whole
			// lines (often with a trailing "\n") as single tokens — so a
			// naive Render(tok.Value) injects spurious trailing spaces that
			// surface as misaligned leading whitespace on the next line
			// (most visibly in diffs). Splitting on "\n" keeps every Render
			// call single-line; raw newlines are re-inserted between.
			parts := strings.Split(tok.Value, "\n")
			for i, part := range parts {
				if i > 0 {
					_, _ = io.WriteString(w, "\n")
				}
				if part == "" {
					continue
				}
				_, _ = io.WriteString(w, s.Render(part))
			}
		}
		return nil
	})
}
