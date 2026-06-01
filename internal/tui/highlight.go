package tui

import (
	"fmt"
	"image/color"
	"io"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// Highlight uses chroma to syntax-highlight source by lang, returning
// an ANSI-colored string. Background is locked to the theme's CardBody
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

	style := paletteChromaStyle(t)
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

	style := paletteChromaStyle(t)
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

// paletteChromaStyle builds a chroma.Style that maps syntax token types to
// the theme's palette colors. This ensures syntax highlighting is visually
// consistent with the palette across all themes (ocean/light/midnight).
func paletteChromaStyle(t Theme) *chroma.Style {
	pal := map[chroma.TokenType]color.Color{
		chroma.Keyword:            t.BrandDeep,
		chroma.KeywordType:        t.BrandDeep,
		chroma.KeywordDeclaration: t.BrandDeep,
		chroma.KeywordNamespace:   t.BrandDeep,
		chroma.NameFunction:       t.BrandDeep,
		chroma.NameClass:          t.BrandDeep,
		chroma.NameBuiltin:        t.BrandDeep,
		chroma.LiteralString:      t.OkColor,
		chroma.Comment:            t.FgFaint,
		chroma.CommentSingle:      t.FgFaint,
		chroma.CommentMultiline:   t.FgFaint,
		chroma.Number:             t.WarnColor,
		chroma.NumberInteger:      t.WarnColor,
		chroma.NumberFloat:        t.WarnColor,
		chroma.Operator:           t.FgMuted,
		chroma.Punctuation:        t.FgMuted,
		chroma.NameTag:            t.AccentFlash,
		chroma.NameAttribute:      t.AccentFlash,
		chroma.Literal:            t.AccentPro,
		chroma.NameVariable:       t.FgBase,
		chroma.Name:               t.FgBase,
		chroma.Error:              t.ErrColor,
	}
	mappings := chroma.StyleEntries{}
	for tok, c := range pal {
		mappings[tok] = chromaColour(c)
	}
	s := chroma.MustNewStyle("palette", mappings)
	return s
}

// chromaColour converts a color.Color to a chroma colour string "#rrggbb".
func chromaColour(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
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
