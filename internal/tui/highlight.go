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

	style := styles.Get("dracula")
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
			_, _ = io.WriteString(w, s.Render(tok.Value))
		}
		return nil
	})
}
