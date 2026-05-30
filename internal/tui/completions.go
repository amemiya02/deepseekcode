// completions.go is the inline popup that backs the `/` and `@` menus.
// It is a small, self-contained list
// component the input owns: not a full-screen overlay but a bordered card
// spliced into View()'s parts slice directly above the input box.
//
// The component holds the full candidate set for the active trigger and a
// fuzzy-filtered index window; ranking reuses fuzzyMatch (fuzzy.go) so the
// menu and the filterable pickers share one matcher. Rows are rendered with
// the raised panel surface so they read as a card consistent with the
// beautify ladder, the cursor row gets the selection band, the fuzzy-matched
// runes in the label are bolded, and the detail column is dimmed.
package tui

import (
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
)

// complMaxRows caps the visible window; longer match sets scroll internally
// with a scrollbar, mirroring crush's 10-row popup.
const complMaxRows = 10

// complItem is one candidate row. insert is the text spliced into the input
// on accept ("/models", "@path/to/file"); label is the left column the fuzzy
// filter matches against and bolds; detail is the dimmed right column
// (typically the command/skill summary); kind tags the row's origin so a
// future renderer can badge built-ins vs. skills.
type complItem struct {
	insert string
	label  string
	detail string
	kind   cmdKind
}

// completions is the popup state machine. items is the immutable candidate
// set for the current trigger; filtered indexes into items after the fuzzy
// pass (sorted by score then label); cursor indexes into filtered. anchorStart
// is the byte offset in the input where the trigger char sits, so the caller
// can replace from there to the cursor on accept.
type completions struct {
	active   bool
	trigger  rune
	query    string
	items    []complItem
	filtered []int // indices into items, fuzzy-ranked
	matches  [][]int
	cursor   int // index into filtered

	anchorStart int

	// capped/maxRows are an external ceiling on the visible row count, set by the
	// layout from the terminal height so the popup never claims more vertical
	// space than the screen can spare above the input box (a short terminal would
	// otherwise overflow the View stack and push the input off-screen). When
	// capped is false (the default, e.g. unit tests that drive the component
	// directly) only the complMaxRows window applies; when capped is true the
	// body shows at most maxRows rows — including 0, which suppresses the popup
	// entirely on a terminal with no room above the input.
	capped  bool
	maxRows int
}

// Open activates the popup for trigger with the given candidate set, recording
// the byte offset of the trigger char. The query starts empty, so filtered is
// the full set (in items order) and the cursor sits at the top.
func (c *completions) Open(trigger rune, items []complItem, anchorStart int) {
	c.active = true
	c.trigger = trigger
	c.items = items
	c.anchorStart = anchorStart
	c.query = ""
	c.cursor = 0
	c.filtered = make([]int, len(items))
	c.matches = make([][]int, len(items))
	for i := range items {
		c.filtered[i] = i
	}
}

// Close deactivates the popup and drops its candidate state so a stale set can
// never render after the trigger is gone.
func (c *completions) Close() {
	c.active = false
	c.items = nil
	c.filtered = nil
	c.matches = nil
	c.query = ""
	c.cursor = 0
	c.anchorStart = 0
}

// Active reports whether the popup is currently open.
func (c *completions) Active() bool { return c.active }

// Trigger returns the rune ('/' or '@') that opened the popup.
func (c *completions) Trigger() rune { return c.trigger }

// AnchorStart returns the byte offset in the input where the trigger char sits.
func (c *completions) AnchorStart() int { return c.anchorStart }

// SetMaxRows bounds the visible body to at most n candidate rows, on top of the
// fixed complMaxRows window. The layout drives this from the terminal height so
// the card (rows + 2 borders) always fits above the input box on short
// terminals. A negative n is treated as 0 — a terminal with no room above the
// input suppresses the popup rather than overflowing the View stack. Once set,
// the cap stays live (it does not reset to "unlimited"); a roomy terminal just
// passes a large n that the complMaxRows window dominates. Because visibleRows
// feeds both Lines and View, the reserved height and the rendered box stay in
// lockstep.
func (c *completions) SetMaxRows(n int) {
	if n < 0 {
		n = 0
	}
	c.capped = true
	c.maxRows = n
}

// SetQuery re-filters the candidate set against q via fuzzyMatch over each
// item's label, sorts the survivors by score (lower is better) then label,
// and resets the cursor to the top. An empty query keeps every item (the
// discovery case) in fuzzyMatch's natural order, broken by label.
func (c *completions) SetQuery(q string) {
	c.query = q

	type scored struct {
		idx     int
		score   int
		matched []int
	}
	hits := make([]scored, 0, len(c.items))
	for i, it := range c.items {
		ok, score, matched := fuzzyMatch(q, it.label)
		if !ok {
			continue
		}
		hits = append(hits, scored{idx: i, score: score, matched: matched})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score < hits[j].score
		}
		return c.items[hits[i].idx].label < c.items[hits[j].idx].label
	})

	c.filtered = make([]int, len(hits))
	c.matches = make([][]int, len(hits))
	for i, h := range hits {
		c.filtered[i] = h.idx
		c.matches[i] = h.matched
	}
	c.cursor = 0
}

// Up moves the cursor toward the top of the filtered set, wrapping to the
// bottom from the first row. A no-op when there are no matches.
func (c *completions) Up() {
	if len(c.filtered) == 0 {
		return
	}
	c.cursor--
	if c.cursor < 0 {
		c.cursor = len(c.filtered) - 1
	}
}

// Down moves the cursor toward the bottom of the filtered set, wrapping to the
// top from the last row. A no-op when there are no matches.
func (c *completions) Down() {
	if len(c.filtered) == 0 {
		return
	}
	c.cursor++
	if c.cursor >= len(c.filtered) {
		c.cursor = 0
	}
}

// Selected returns the item under the cursor and true, or the zero item and
// false when inactive or no candidate matches the query.
func (c *completions) Selected() (complItem, bool) {
	if !c.active || len(c.filtered) == 0 {
		return complItem{}, false
	}
	if c.cursor < 0 || c.cursor >= len(c.filtered) {
		return complItem{}, false
	}
	return c.items[c.filtered[c.cursor]], true
}

// Lines reports the exact number of terminal rows View currently occupies, so
// View()'s layout can reserve that height for the body. It equals the newline
// count in View's output plus one (or 0 when View renders nothing). Because it
// is derived from the same window math as View, the two cannot drift.
func (c *completions) Lines() int {
	n := c.visibleRows()
	if n == 0 {
		return 0
	}
	// rows + top border + bottom border.
	return n + 2
}

// visibleRows returns how many candidate rows the popup body shows: 0 when
// inactive or nothing matches, otherwise min(matches, complMaxRows) further
// bounded by the layout's terminal-height ceiling (maxRows) when one is set. On
// a short terminal the popup shrinks (and scrolls the overflow) instead of
// pushing the input box off-screen; with no room at all the ceiling is 0 and the
// popup is suppressed entirely.
func (c *completions) visibleRows() int {
	if !c.active || len(c.filtered) == 0 {
		return 0
	}
	limit := complMaxRows
	if c.capped && c.maxRows < limit {
		limit = c.maxRows
	}
	if limit < 0 {
		limit = 0
	}
	if len(c.filtered) > limit {
		return limit
	}
	return len(c.filtered)
}

// window returns the [start, end) slice of filtered indices the popup shows,
// scrolling to keep the cursor in view when the match set exceeds the cap.
func (c *completions) window() (int, int) {
	n := c.visibleRows()
	if n == 0 {
		return 0, 0
	}
	if len(c.filtered) <= n {
		return 0, len(c.filtered)
	}
	// Center the cursor in the window, clamped to the ends.
	start := c.cursor - n/2
	if start < 0 {
		start = 0
	}
	if start > len(c.filtered)-n {
		start = len(c.filtered) - n
	}
	return start, start + n
}

// View renders the popup as a raised, bordered card. It returns "" when the
// popup is inactive or no candidate matches the query. Otherwise it shows up
// to complMaxRows rows: each row is "<label>  <detail>", the fuzzy-matched
// runes in the label are bolded, the detail column is dimmed, and the cursor
// row is drawn as the selection band. When the match set exceeds the cap a
// scrollbar rides the right edge and the window scrolls around the cursor.
func (c *completions) View(t Theme, width int) string {
	if c.visibleRows() == 0 {
		// Inactive, no match, or clamped to zero rows by the layout's
		// terminal-height ceiling — render nothing so Lines() (which also routes
		// through visibleRows) and View stay in lockstep.
		return ""
	}

	// Interior width: the card border eats 2 cells; clamp to a sane minimum so
	// a very narrow terminal still produces a usable box (the box may then
	// exceed the terminal, but a sub-24-cell popup is illegible anyway).
	inner := width - 2
	if inner < 24 {
		inner = 24
	}

	start, end := c.window()
	scroll := len(c.filtered) > c.visibleRows()
	rowW := inner
	if scroll {
		rowW-- // reserve the rightmost cell for the scrollbar glyph
	}

	dim := lipgloss.NewStyle().Foreground(t.FgFaint)
	bold := lipgloss.NewStyle().Bold(true)

	var thumbStart, thumbEnd int
	if scroll {
		// Track the window's scroll position (its start row), not the cursor:
		// the fraction is start / (total - visible), so the thumb sits at the
		// top when the window is at the top and the bottom when it's at the
		// bottom, instead of drifting with the cursor inside a static window.
		pct := 0.0
		if denom := len(c.filtered) - (end - start); denom > 0 {
			pct = float64(start) / float64(denom)
		}
		thumbStart, thumbEnd = scrollbarThumb(end-start, len(c.filtered), pct)
	}

	rows := make([]string, 0, end-start)
	for vi, fi := 0, start; fi < end; vi, fi = vi+1, fi+1 {
		it := c.items[c.filtered[fi]]
		text := c.renderRow(it, c.matches[fi], rowW, fi == c.cursor, t, dim, bold)
		if scroll {
			glyph := lipgloss.NewStyle().Foreground(t.BorderColor).Render(ScrollbarTrack)
			if vi >= thumbStart && vi < thumbEnd {
				glyph = lipgloss.NewStyle().Foreground(t.BrandLight).Render(ScrollbarThumb)
			}
			text += glyph
		}
		rows = append(rows, text)
	}

	// Each row is already composed to exactly `inner` display cells (content
	// padded to rowW plus the optional 1-cell scrollbar glyph). We therefore
	// apply only the border + surface to the joined body and DO NOT set a
	// Width on the card — setting one would re-wrap a row whose styled fill
	// already spans the full interior, splitting it across two lines.
	body := strings.Join(rows, "\n")
	card := t.Panel(TierRaised).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderColor)
	return card.Render(body)
}

// renderRow composes one candidate line at width rowW: the label with its
// fuzzy-matched runes bolded, a two-space gap, then the dimmed detail column,
// truncated to fit. The cursor row is wrapped in the selection band so it
// reads the same way as the picker overlays.
func (c *completions) renderRow(it complItem, matched []int, rowW int, selected bool, t Theme, dim, bold lipgloss.Style) string {
	label := boldMatched(it.label, matched, bold)
	line := label
	if it.detail != "" {
		// Budget the detail column by what the label leaves, minus a 2-cell gap.
		gap := rowW - lipgloss.Width(label) - 2
		if gap > 0 {
			detail := it.detail
			if dw := lipgloss.Width(detail); dw > gap {
				detail = truncateCells(detail, gap)
			}
			line += "  " + dim.Render(detail)
		}
	}

	if selected {
		return selectedRow(t, sanitizeRow(it, matched, bold), rowW)
	}
	// Pad to the full row width so the panel background fills the line.
	return lipgloss.NewStyle().Width(rowW).Render(line)
}

// sanitizeRow renders the cursor row's text WITHOUT per-rune styling: the
// selection band paints its own bold-on-accent foreground, so embedding bold
// SGR escapes inside it would reset the band's color mid-row. We therefore
// hand selectedRow plain text (label + gap + detail), letting the band own the
// styling. matched/bold are accepted for symmetry with renderRow but unused.
func sanitizeRow(it complItem, _ []int, _ lipgloss.Style) string {
	line := it.label
	if it.detail != "" {
		line += "  " + it.detail
	}
	return line
}

// boldMatched returns label with the runes at the given indices wrapped in the
// bold style. Indices are rune indices into label (as fuzzyMatch returns); out
// of range indices are skipped defensively.
func boldMatched(label string, matched []int, bold lipgloss.Style) string {
	if len(matched) == 0 {
		return label
	}
	hit := make(map[int]bool, len(matched))
	for _, m := range matched {
		hit[m] = true
	}
	var b strings.Builder
	for i, r := range []rune(label) {
		if hit[i] {
			b.WriteString(bold.Render(string(r)))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// truncateCells trims s to at most n display cells, appending a single-cell
// ellipsis when it cuts. It operates on runes (the detail column is plain
// unstyled text at the call site) and is a no-op when s already fits.
func truncateCells(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= n {
		return s
	}
	if n == 1 {
		return "…"
	}
	rs := []rune(s)
	w := 0
	cut := len(rs)
	for i, r := range rs {
		rw := lipgloss.Width(string(r))
		if w+rw > n-1 { // leave room for the ellipsis
			cut = i
			break
		}
		w += rw
	}
	return string(rs[:cut]) + "…"
}
