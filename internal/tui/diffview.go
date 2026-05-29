package tui

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

// diffview renders a UNIFIED diff (no split view) with:
//   - a left line-number gutter (fgFaint on bgWell),
//   - background-filled +/- bands using the hard-coded diff colors
//     (theme.DiffAdd* / theme.DiffDel*),
//   - equal/context lines as fgMuted on bgWell,
//   - syntax highlighting routed through the existing chroma path, with each
//     line's background FORCED to its band/well color so the highlight sits on
//     the filled band rather than the terminal default.
//
// It is the rendering target for tool-result diffs (edit / write / apply_patch)
// and the structured git_diff tool output, replacing the plain chroma "diff"
// lexer for those payloads.

// diffLineKind classifies one physical line of a unified diff.
type diffLineKind int

const (
	diffEqual diffLineKind = iota // context line (leading space, or bare)
	diffAdd                       // "+" line
	diffDel                       // "-" line
	diffHunk                      // "@@ ... @@" hunk header
	diffMeta                      // "diff --git", "index", "+++", "---", etc.
)

// gutterWidth is the fixed width of the left line-number column (not counting
// the trailing separator space). Five digits covers files up to 99,999 lines;
// wider numbers simply overflow the column without breaking layout.
const diffGutterWidth = 5

// renderDiff renders src (a unified diff) for the given theme at the given
// total width. The returned string is a sequence of "\n"-joined lines, each
// already prefixed with the shared 2-cell gutter so it aligns with other
// barred/gutter items. Caller appends its own header.
//
// width is the full item width; the code area is width minus the shared gutter
// minus the line-number column and its separator.
func renderDiff(t Theme, src, lang string, width int) string {
	if src == "" {
		return ""
	}
	if c, ok := diffCacheGet(t, src, lang, width); ok {
		return c
	}
	out := renderDiffUncached(t, src, lang, width)
	diffCachePut(t, src, lang, width, out)
	return out
}

func renderDiffUncached(t Theme, src, lang string, width int) string {
	lines := strings.Split(strings.TrimRight(src, "\n"), "\n")

	// Code column width: total minus shared gutter (2) minus number gutter
	// (diffGutterWidth) minus the single separator space between them.
	codeWidth := width - len(t.Gutter()) - diffGutterWidth - 1
	if codeWidth < 1 {
		codeWidth = 1
	}

	// Running line numbers for the two sides, seeded from each hunk header.
	var oldLine, newLine int

	var b strings.Builder
	for _, raw := range lines {
		kind := classifyDiffLine(raw)

		switch kind {
		case diffHunk:
			oldLine, newLine = parseHunkHeader(raw)
			b.WriteString(renderHunkHeader(t, raw, width))
			b.WriteString("\n")
			continue
		case diffMeta:
			b.WriteString(renderMetaLine(t, raw, width))
			b.WriteString("\n")
			continue
		}

		// Code-bearing line (equal/add/del): strip the +/-/space marker column
		// once, then pick the line number from the relevant side.
		var num int
		code := stripMarker(raw)
		switch kind {
		case diffAdd:
			num = newLine
			newLine++
		case diffDel:
			num = oldLine
			oldLine++
		default: // diffEqual
			num = newLine
			oldLine++
			newLine++
		}

		b.WriteString(renderCodeLine(t, kind, num, code, lang, codeWidth))
		b.WriteString("\n")
	}
	return b.String()
}

// classifyDiffLine maps one raw diff line to a diffLineKind. The order of
// checks matters: hunk and meta markers are recognised before the bare +/-
// content lines so "+++ b/file" / "--- a/file" headers are treated as meta,
// not as add/del code.
func classifyDiffLine(s string) diffLineKind {
	switch {
	case strings.HasPrefix(s, "@@"):
		return diffHunk
	case strings.HasPrefix(s, "+++"), strings.HasPrefix(s, "---"):
		return diffMeta
	case strings.HasPrefix(s, "diff --git "),
		strings.HasPrefix(s, "index "),
		strings.HasPrefix(s, "new file"),
		strings.HasPrefix(s, "deleted file"),
		strings.HasPrefix(s, "old mode"),
		strings.HasPrefix(s, "new mode"),
		strings.HasPrefix(s, "rename "),
		strings.HasPrefix(s, "copy "),
		strings.HasPrefix(s, "similarity "),
		strings.HasPrefix(s, "dissimilarity "),
		strings.HasPrefix(s, "Binary files "),
		strings.HasPrefix(s, "\\ No newline"):
		return diffMeta
	case strings.HasPrefix(s, "+"):
		return diffAdd
	case strings.HasPrefix(s, "-"):
		return diffDel
	default:
		return diffEqual
	}
}

// stripMarker removes the leading +/-/space column from a code-bearing diff
// line, returning the underlying source line. A bare line (no marker) is
// returned unchanged.
func stripMarker(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '+', '-', ' ':
		return s[1:]
	default:
		return s
	}
}

// parseHunkHeader extracts the starting old/new line numbers from a unified
// hunk header of the form "@@ -oldStart,oldCount +newStart,newCount @@ ...".
// On any parse failure it returns (1, 1) so rendering still produces sane,
// monotonically-increasing numbers.
func parseHunkHeader(s string) (oldStart, newStart int) {
	oldStart, newStart = 1, 1
	// Trim to the segment between the first "@@" pair.
	body := strings.TrimPrefix(s, "@@")
	if idx := strings.Index(body, "@@"); idx >= 0 {
		body = body[:idx]
	}
	for _, tok := range strings.Fields(body) {
		if len(tok) < 2 {
			continue
		}
		switch tok[0] {
		case '-':
			oldStart = parseLineStart(tok[1:])
		case '+':
			newStart = parseLineStart(tok[1:])
		}
	}
	return oldStart, newStart
}

// parseLineStart parses the "start" of a "start,count" hunk range token.
func parseLineStart(tok string) int {
	if idx := strings.IndexByte(tok, ','); idx >= 0 {
		tok = tok[:idx]
	}
	n, err := strconv.Atoi(tok)
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// renderCodeLine renders one code-bearing diff line: the line-number gutter
// (fgFaint on bgWell) followed by a single separator space then the
// background-filled code band. Add/del bands use the hard-coded diff colors;
// equal lines use fgMuted on bgWell. The code text is syntax-highlighted
// through chroma with its background FORCED to the band color so the highlight
// sits on the fill.
func renderCodeLine(t Theme, kind diffLineKind, num int, code, lang string, codeWidth int) string {
	bandFg, bandBg := diffBandColors(t, kind)

	// Gutter: blank the number for add lines on the old side / del lines on the
	// new side is not modelled (unified single-number column) — we show the
	// side-appropriate number computed by the caller.
	gut := renderNumberGutter(t, num)

	// Highlight the code on the band background. When highlighting is disabled
	// (no lang, degraded mode), fall back to a plain band-styled render.
	codeCell := renderCodeCell(t, code, lang, bandFg, bandBg, codeWidth)

	return t.Gutter() + gut + " " + codeCell
}

// renderNumberGutter renders the right-aligned line number in fgFaint on
// bgWell, padded to diffGutterWidth. It honours degraded mode by dropping the
// background fill (the foreground-only number still reads as a gutter).
func renderNumberGutter(t Theme, num int) string {
	label := strconv.Itoa(num)
	if num <= 0 {
		label = ""
	}
	if len(label) < diffGutterWidth {
		label = strings.Repeat(" ", diffGutterWidth-len(label)) + label
	}
	style := lipgloss.NewStyle().Foreground(t.FgFaint)
	if fillsEnabled(t) {
		style = style.Background(t.BgWell)
	}
	return style.Render(label)
}

// renderCodeCell renders the code text for one band: it syntax-highlights via
// chroma (background forced to bandBg) when lang is set and fills are enabled,
// otherwise it renders the raw text in bandFg. Either way the cell is padded
// to codeWidth so the band fill extends to the right edge.
func renderCodeCell(t Theme, code, lang string, bandFg, bandBg color.Color, codeWidth int) string {
	// Clip overly long code to the column so the band stays one row tall.
	display := code
	if w := lipgloss.Width(display); w > codeWidth {
		display = clipToWidth(display, codeWidth)
	}

	var inner string
	if lang != "" && fillsEnabled(t) {
		// Force the chroma background to the band color so highlighting sits on
		// the filled band. highlightLineOnBg returns a single ANSI-styled line
		// already backgrounded; we then pad to width on the same background.
		inner = highlightLineOnBg(t, display, lang, bandBg)
	} else {
		// No highlighting: foreground = band fg.
		fgStyle := lipgloss.NewStyle().Foreground(bandFg)
		if fillsEnabled(t) {
			fgStyle = fgStyle.Background(bandBg)
		}
		inner = fgStyle.Render(display)
	}

	// Pad the remainder of the column with the band background so the fill
	// reaches the right edge. In degraded mode there's no fill to extend.
	used := lipgloss.Width(inner)
	if pad := codeWidth - used; pad > 0 {
		padStyle := lipgloss.NewStyle()
		if fillsEnabled(t) {
			padStyle = padStyle.Background(bandBg)
		}
		inner += padStyle.Render(strings.Repeat(" ", pad))
	}
	return inner
}

// diffBandColors returns the foreground / background colors for a code-bearing
// line of the given kind. Add/del use the hard-coded diff bands; equal lines
// use fgMuted on bgWell.
func diffBandColors(t Theme, kind diffLineKind) (fg, bg color.Color) {
	switch kind {
	case diffAdd:
		return t.DiffAddFg, t.DiffAddBg
	case diffDel:
		return t.DiffDelFg, t.DiffDelBg
	default:
		return t.FgMuted, t.BgWell
	}
}

// renderHunkHeader renders a "@@ ... @@" hunk header as a full-width band:
// brandLight foreground on bgRaised, padded across the code+gutter span so it
// reads as a section divider.
func renderHunkHeader(t Theme, raw string, width int) string {
	style := lipgloss.NewStyle().Foreground(t.BrandLight).Bold(true)
	if fillsEnabled(t) {
		style = style.Background(t.BgRaised)
	}
	inner := width - len(t.Gutter())
	if inner < 1 {
		inner = 1
	}
	text := raw
	if lipgloss.Width(text) > inner {
		text = clipToWidth(text, inner)
	}
	if pad := inner - lipgloss.Width(text); pad > 0 && fillsEnabled(t) {
		text += strings.Repeat(" ", pad)
	}
	return t.Gutter() + style.Render(text)
}

// renderMetaLine renders a diff metadata line (file headers, mode changes,
// "\ No newline", etc.) in fgFaint on bgWell — recessed, secondary to the
// code bands.
func renderMetaLine(t Theme, raw string, width int) string {
	style := lipgloss.NewStyle().Foreground(t.FgFaint)
	if fillsEnabled(t) {
		style = style.Background(t.BgWell)
	}
	inner := width - len(t.Gutter())
	if inner < 1 {
		inner = 1
	}
	text := raw
	if lipgloss.Width(text) > inner {
		text = clipToWidth(text, inner)
	}
	if pad := inner - lipgloss.Width(text); pad > 0 && fillsEnabled(t) {
		text += strings.Repeat(" ", pad)
	}
	return t.Gutter() + style.Render(text)
}

// fillsEnabled reports whether background fills should be drawn: true only when
// the theme is neither transparent nor degraded to non-truecolor. This mirrors
// Theme.Panel's gating so diff bands degrade in lockstep with panels.
func fillsEnabled(t Theme) bool {
	return !t.Transparent() && t.Truecolor()
}

// clipToWidth truncates s (which may contain wide runes but no ANSI) to at most
// w display cells, appending a single-cell ellipsis when it overflows.
func clipToWidth(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	// Build up rune-by-rune until we'd exceed w-1 (reserve a cell for "…").
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if used+rw > w-1 {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	b.WriteString("…")
	return b.String()
}

// highlightLineOnBg syntax-highlights a single source line through chroma with
// the background FORCED to bg, returning the ANSI-styled line. It reuses the
// shared chromaFormatter so token→color mapping matches the rest of the TUI;
// the only difference from Highlight is the fixed background and single-line
// scope. On any lexing failure it falls back to a plain bg-styled render.
func highlightLineOnBg(t Theme, line, lang string, bg color.Color) string {
	if line == "" {
		// Empty code still needs the band background for the (later) padding to
		// be contiguous; return empty and let the caller pad.
		return ""
	}
	h := highlightOnBg(t, line, lang, bg)
	// chroma may append a trailing newline for a single line; trim it so the
	// caller's width math (and the band) stay single-row.
	h = strings.TrimRight(h, "\n")
	if h == "" {
		style := lipgloss.NewStyle().Foreground(t.FgBase).Background(bg)
		return style.Render(line)
	}
	return h
}

// diffCache memoises rendered diffs keyed by hash(theme-identity + content +
// lang + width). Diffs are expensive to re-highlight every frame; the cache is
// a small LRU shared across the package, reset alongside the render cache when
// the scrollback clears.
var diffCache = NewRenderCache(64)

func diffCacheKey(t Theme, src, lang string, width int) string {
	// Theme identity for the key: name + the two flags that change fills +
	// the band colors (so a theme swap invalidates). Hash the content to keep
	// the key bounded.
	sum := sha256.Sum256([]byte(src))
	var h uint64 = binary.BigEndian.Uint64(sum[:8])
	return fmt.Sprintf("diff:%s:%t:%t:%s:%d:%016x:%d",
		t.Name, t.Transparent(), t.Truecolor(), lang, width, h, len(src))
}

func diffCacheGet(t Theme, src, lang string, width int) (string, bool) {
	return diffCache.Get(diffCacheKey(t, src, lang, width))
}

func diffCachePut(t Theme, src, lang string, width int, rendered string) {
	diffCache.Put(diffCacheKey(t, src, lang, width), rendered)
}
