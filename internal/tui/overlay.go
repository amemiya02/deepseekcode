package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/amemiya02/deepseekcode/internal/session"
)

// overlayMode is the App's display mode. chat is the default; the
// pickers and tape view replace the main view fullscreen until ESC.
type overlayMode int

const (
	modeChat     overlayMode = iota
	modeTape                 // /tape — Reasoning Tape fullscreen view
	modeModels               // /models — model picker (filterable, G6)
	modeSessions             // /sessions — session tree picker (filterable, G6)
	modePalette              // ctrl+p — fuzzy command palette (G5)
	modeHelp                 // ? / ctrl+g / /help — keybinding + command overlay (G7)
)

// Overlay owns the modal-picker state — which picker is up, where
// the cursor sits, and the picker-specific data rows. App composes
// one of these instead of carrying overlay/overlayCursor/models/
// sessionsRows as four flat fields.
//
// The filterable pickers (/models, /sessions) and the command palette share a
// single filterableList (G5/G6): it holds the live filter string and the
// fuzzy-ranked subset of visible row indices, so the cursor moves through the
// narrowed set rather than the raw rows. modeTape keeps the old direct-cursor
// model (no filter); modeHelp keeps a plain scroll offset.
type Overlay struct {
	mode         overlayMode
	cursor       int // tape entry index / help scroll offset
	models       []modelOption
	sessionsRows []sessionRow
	palette      []paletteAction
	filter       filterableList
}

// NewOverlay returns an idle Overlay (mode = chat, nothing open).
func NewOverlay() *Overlay { return &Overlay{mode: modeChat} }

// Mode returns the active overlay mode (modeChat when none is open).
func (o *Overlay) Mode() overlayMode { return o.mode }

// IsOpen reports whether a picker is currently visible.
func (o *Overlay) IsOpen() bool { return o.mode != modeChat }

// Cursor returns the picker's current cursor index.
func (o *Overlay) Cursor() int { return o.cursor }

// Close returns to the chat view.
func (o *Overlay) Close() { o.mode = modeChat; o.cursor = 0; o.filter = filterableList{} }

// OpenTape switches to the /tape view with the cursor at the top.
func (o *Overlay) OpenTape() { o.mode = modeTape; o.cursor = 0 }

// OpenModels switches to the /models picker. The filterableList is seeded with
// one label per model (short name + note); the cursor lands on the row that
// matches activeID, falling back to 0 if no match. Once the user filters, the
// cursor tracks the narrowed set.
func (o *Overlay) OpenModels(activeID string) {
	o.models = availableModels()
	o.mode = modeModels
	o.cursor = 0
	labels := make([]string, len(o.models))
	for i, m := range o.models {
		labels[i] = m.Short + " " + m.Note
	}
	o.filter.SetRows(labels)
	for i, m := range o.models {
		if m.ID == activeID {
			o.cursorTo(i)
			break
		}
	}
}

// OpenSessions switches to the /sessions picker with the supplied rows. The
// filter matches against the short id + summary of each row.
func (o *Overlay) OpenSessions(rows []sessionRow) {
	o.sessionsRows = rows
	o.mode = modeSessions
	o.cursor = 0
	labels := make([]string, len(rows))
	for i, r := range rows {
		labels[i] = r.Sess.ID + " " + r.Sess.Summary
	}
	o.filter.SetRows(labels)
}

// OpenPalette switches to the command palette (G5) over the supplied action
// list. The filter starts empty so every action is visible.
func (o *Overlay) OpenPalette(actions []paletteAction) {
	o.palette = actions
	o.mode = modePalette
	o.cursor = 0
	labels := make([]string, len(actions))
	for i, a := range actions {
		labels[i] = a.label
	}
	o.filter.SetRows(labels)
}

// OpenHelp switches to the help overlay (G7), scrolled to the top.
func (o *Overlay) OpenHelp() { o.mode = modeHelp; o.cursor = 0 }

// Filterable reports whether the active overlay routes typing into the shared
// filter (the pickers and the palette) rather than treating letters as nav
// keys. modeTape and modeHelp are not filterable.
func (o *Overlay) Filterable() bool {
	switch o.mode {
	case modeModels, modeSessions, modePalette:
		return true
	}
	return false
}

// MoveDown / MoveUp advance the cursor inside the active overlay. For the
// filterable pickers they walk the visible (narrowed) set; modeTape and
// modeHelp move the raw cursor / scroll offset.
func (o *Overlay) MoveDown() {
	if o.Filterable() {
		o.filter.Down()
		return
	}
	o.cursor++
}

func (o *Overlay) MoveUp() {
	if o.Filterable() {
		o.filter.Up()
		return
	}
	if o.cursor > 0 {
		o.cursor--
	}
}

// FilterType / FilterBackspace / FilterClear drive the shared filter for the
// filterable overlays. FilterClear reports whether there was a filter to clear
// (so the caller can implement "esc clears, then closes").
func (o *Overlay) FilterType(r rune)    { o.filter.Type(r) }
func (o *Overlay) FilterBackspace()     { o.filter.Backspace() }
func (o *Overlay) FilterClear() bool    { return o.filter.ClearFilter() }
func (o *Overlay) FilterString() string { return o.filter.Filter() }
func (o *Overlay) VisibleRows() []int   { return o.filter.Visible() }
func (o *Overlay) FilterCursor() int    { return o.filter.Cursor() }

// cursorTo moves the filter cursor to the visible position that maps to the
// original row index, when present. Used to preselect the active model row.
func (o *Overlay) cursorTo(rowIdx int) {
	for vi, ri := range o.filter.Visible() {
		if ri == rowIdx {
			o.filter.cursor = vi
			return
		}
	}
}

// Models / SessionsRows / Palette return the picker-specific row slices for
// rendering. Returned slices are read-only.
func (o *Overlay) Models() []modelOption      { return o.models }
func (o *Overlay) SessionsRows() []sessionRow { return o.sessionsRows }
func (o *Overlay) Palette() []paletteAction   { return o.palette }

// SelectedModelID returns the model id under the cursor (mapped through the
// filter), or "" when nothing matches.
func (o *Overlay) SelectedModelID() string {
	if i := o.filter.Selected(); i >= 0 && i < len(o.models) {
		return o.models[i].ID
	}
	return ""
}

// SelectedSessionID returns the session id under the cursor (mapped through
// the filter), or "" when nothing matches.
func (o *Overlay) SelectedSessionID() string {
	if i := o.filter.Selected(); i >= 0 && i < len(o.sessionsRows) {
		return o.sessionsRows[i].Sess.ID
	}
	return ""
}

// SelectedAction returns the palette action under the cursor and true, or the
// zero action and false when nothing matches the filter.
func (o *Overlay) SelectedAction() (paletteAction, bool) {
	if i := o.filter.Selected(); i >= 0 && i < len(o.palette) {
		return o.palette[i], true
	}
	return paletteAction{}, false
}

// modelOption is one row in the /models picker.
type modelOption struct {
	ID    string
	Short string
	Note  string // e.g. pricing summary
}

// availableModels returns the picker rows for /models. Pricing is
// hard-coded here (mirrors internal/llm/cache_metrics.go) so the
// picker is informative even offline.
func availableModels() []modelOption {
	return []modelOption{
		{ID: "deepseek-v4-flash", Short: "flash", Note: "1M ctx · ¥1/¥0.02 in · ¥2 out · default"},
		{ID: "deepseek-v4-pro", Short: "pro", Note: "1M ctx · ¥3/¥0.025 in · ¥6 out · stronger"},
		{ID: "deepseek-chat", Short: "chat", Note: "1M ctx · ¥1/¥0.02 in · ¥2 out · alias → flash"},
		{ID: "deepseek-reasoner", Short: "reasoner", Note: "1M ctx · ¥1/¥0.02 in · ¥2 out · alias → flash (thinking)"},
	}
}

// sessionRow is one row in the /sessions picker.
type sessionRow struct {
	Sess   session.Session
	Indent int // depth in the parent chain (for tree rendering)
}

// renderTape returns the fullscreen /tape view body. Each reasoning
// block + tool call + hook execution renders as one tape entry with
// model attribution glyph.
//
// Cursor is the index of the currently focused tape entry; j/k moves it.
func renderTape(t Theme, items []chatItem, cursor int, width, height int) string {
	tape := tapeEntries(items)
	if len(tape) == 0 {
		body := t.Hint.Render("(no tape entries yet — let the agent think and act first)")
		return wrapPane(t, "/tape", "no entries", body, width, height)
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= len(tape) {
		cursor = len(tape) - 1
	}
	rowW := width - 8 // inside border (2) + padding (4) + a small margin
	if rowW < 20 {
		rowW = 20
	}
	var b strings.Builder
	for i, e := range tape {
		if i == cursor {
			// Selected entry: brandDeep bg-fill with onAccent fg, plus ▶ marker.
			b.WriteString(selectedRow(t, "▶ "+e.glyph+" "+e.label, rowW))
		} else {
			b.WriteString("  ")
			b.WriteString(e.renderLine(t))
		}
		b.WriteByte('\n')
	}
	header := fmt.Sprintf("%d entries · cursor %d/%d", len(tape), cursor+1, len(tape))
	return wrapPane(t, "/tape", header, b.String(), width, height)
}

// tapeEntry is the projection of chatItems onto the tape's visible
// timeline (reasoning blocks, tool calls, hook executions).
type tapeEntry struct {
	kind       itemKind
	glyph      string // ◇ flash / ◆ pro
	label      string
	expandable bool
	chatIdx    int // back-reference into App.items
}

func tapeEntries(items []chatItem) []tapeEntry {
	var out []tapeEntry
	for i, it := range items {
		switch it.kind {
		case itemReasoning:
			out = append(out, tapeEntry{
				kind:       it.kind,
				glyph:      "◇",
				label:      fmt.Sprintf("flash  reasoning  %.1fs  ~%d tok", it.duration.Seconds(), it.tokens),
				expandable: true,
				chatIdx:    i,
			})
		case itemToolCall:
			out = append(out, tapeEntry{
				kind:    it.kind,
				glyph:   "◇",
				label:   fmt.Sprintf("flash  tool_call  %s", it.tool),
				chatIdx: i,
			})
		case itemRepair:
			var toolPart string
			if it.repairTool != "" {
				toolPart = it.repairTool + " "
			}
			out = append(out, tapeEntry{
				kind:    it.kind,
				glyph:   "◈",
				label:   fmt.Sprintf("repair  %s%s", toolPart, it.repairMessage),
				chatIdx: i,
			})
		}
	}
	return out
}

func (e tapeEntry) renderLine(t Theme) string {
	return t.ToolCall.Render(e.glyph) + " " + t.ToolCall.Render(e.label)
}

// filterLine renders the one-line filter input at the top of a filterable
// picker/palette body (G5/G6): a dimmed "filter " label, the live query, and a
// caret. An empty query shows a dimmed hint so the affordance is discoverable.
func filterLine(t Theme, query string) string {
	if query == "" {
		return t.Hint.Render("filter: ") + t.Hint.Render("(type to narrow)")
	}
	return t.Hint.Render("filter: ") + t.StatusModel.Render(query) + t.Hint.Render("▏")
}

// renderModelsPicker draws the /models picker overlay. visible is the
// fuzzy-narrowed row order (indices into models); cursor is the position
// within visible. A filter input rides above the rows (G6).
func renderModelsPicker(t Theme, models []modelOption, visible []int, cursor int, filter, activeID string, width, height int) string {
	rowW := width - 8 // inside border (2) + padding (4) + a small margin
	if rowW < 20 {
		rowW = 20
	}
	var b strings.Builder
	b.WriteString(filterLine(t, filter) + "\n\n")
	if len(visible) == 0 {
		b.WriteString(t.Hint.Render("(no models match the filter)"))
	}
	for vi, idx := range visible {
		m := models[idx]
		active := " "
		if m.ID == activeID {
			active = "*"
		}
		if vi == cursor {
			// Selected row: brandDeep bg-fill with onAccent fg spanning the
			// row, in addition to the ▶ marker. Plain text inside the filled
			// style so the foreground stays legible on the accent band.
			text := fmt.Sprintf("▶ %s %s  %s", active, m.Short, m.Note)
			b.WriteString(selectedRow(t, text, rowW) + "\n")
		} else {
			marker := " "
			if m.ID == activeID {
				marker = t.StatusGood.Render("*")
			}
			line := fmt.Sprintf("  %s %s  %s", marker, t.StatusModel.Render(m.Short), t.Hint.Render(m.Note))
			b.WriteString(line + "\n")
		}
	}
	header := fmt.Sprintf("%d models · type to filter · ⏎ switch · esc cancel", len(models))
	return wrapPane(t, "/models", header, b.String(), width, height)
}

// renderSessionsPicker draws the /sessions picker overlay. visible is the
// fuzzy-narrowed row order (indices into rows); cursor is the position within
// visible. A filter input rides above the rows (G6).
func renderSessionsPicker(t Theme, rows []sessionRow, visible []int, cursor int, filter, activeID string, width, height int) string {
	if len(rows) == 0 {
		body := t.Hint.Render("(no sessions in this project yet)")
		return wrapPane(t, "/sessions", "0 sessions", body, width, height)
	}
	rowW := width - 8 // inside border (2) + padding (4) + a small margin
	if rowW < 20 {
		rowW = 20
	}
	var b strings.Builder
	b.WriteString(filterLine(t, filter) + "\n\n")
	if len(visible) == 0 {
		b.WriteString(t.Hint.Render("(no sessions match the filter)"))
		header := fmt.Sprintf("%d sessions · type to filter · ⏎ switch · esc cancel", len(rows))
		return wrapPane(t, "/sessions", header, b.String(), width, height)
	}

	view := listBodyHeight(height)
	start, end := listWindow(cursor, len(visible), view)
	scroll := len(visible) > view
	if scroll {
		rowW-- // reserve the rightmost cell for the scrollbar glyph
	}
	var thumbStart, thumbEnd int
	if scroll {
		pct := 0.0
		if denom := len(visible) - (end - start); denom > 0 {
			pct = float64(start) / float64(denom)
		}
		thumbStart, thumbEnd = scrollbarThumb(end-start, len(visible), pct)
	}

	for vi := start; vi < end; vi++ {
		r := rows[visible[vi]]
		indent := strings.Repeat("  ", r.Indent)
		summary := r.Sess.Summary
		if summary == "" {
			summary = "(no summary)"
		}
		shortID := r.Sess.ID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		when := r.Sess.LastUsedAt.Local().Format("2006-01-02 15:04")
		active := " "
		if r.Sess.ID == activeID {
			active = "*"
		}
		var row string
		if vi == cursor {
			// Selected row: brandDeep bg-fill with onAccent fg, plus ▶ marker.
			text := fmt.Sprintf("▶ %s %s%s  %s  %s", active, indent, shortID, when, summary)
			row = selectedRow(t, truncateCells(text, rowW), rowW)
		} else {
			marker := " "
			if r.Sess.ID == activeID {
				marker = t.StatusGood.Render("*")
			}
			// Budget the summary so the row stays one line. truncateCells can't
			// run on the ANSI-styled shortID, so the (unstyled) summary absorbs
			// the trim using the plain prefix's display width.
			prefix := fmt.Sprintf("  %s %s%s  %s  ", active, indent, shortID, when)
			rem := rowW - lipgloss.Width(prefix)
			if rem < 0 {
				rem = 0
			}
			line := fmt.Sprintf("  %s %s%s  %s  %s",
				marker, indent, t.StatusModel.Render(shortID), when, truncateCells(summary, rem))
			row = lipgloss.NewStyle().Width(rowW).Render(line)
		}
		if scroll {
			row += scrollGlyph(t, vi-start, thumbStart, thumbEnd)
		}
		b.WriteString(row + "\n")
	}
	header := fmt.Sprintf("%d sessions · type to filter · ⏎ switch · esc cancel", len(rows))
	if scroll {
		header = fmt.Sprintf("%d sessions · %d–%d · type to filter · ⏎ switch · esc cancel", len(rows), start+1, end)
	}
	return wrapPane(t, "/sessions", header, b.String(), width, height)
}

// paletteAction is one row in the command palette (G5). id is the stable verb
// the dispatcher routes on (a slash command like "/models" or a non-slash verb
// like "toggle-thinking"); label is the displayed, fuzzy-matched text; detail
// is the dimmed right-hand summary. Keeping id distinct from label means a
// relabel can never silently misroute a verb.
type paletteAction struct {
	id     string
	label  string
	detail string
}

// listWindow returns the [start, end) slice of an n-row list to display in a
// viewport `rows` tall, scrolled so `cursor` stays visible (centred when it can
// be, clamped to the ends). rows<=0 or n==0 yields an empty window. This is what
// lets a long palette / session list scroll under ↑/↓ instead of always
// rendering from the top and clipping — the bug where ↓ advanced an off-screen
// cursor and the view never followed it.
func listWindow(cursor, n, rows int) (int, int) {
	if rows <= 0 || n == 0 {
		return 0, 0
	}
	if n <= rows {
		return 0, n
	}
	start := cursor - rows/2
	if start < 0 {
		start = 0
	}
	if start > n-rows {
		start = n - rows
	}
	return start, start + rows
}

// listBodyHeight returns how many list rows a wrapPane body can show inside a
// pane `height` tall: the pane chrome (border 2 + vertical padding 2 + title 1)
// and the filter input plus its blank separator (2) come off the top.
func listBodyHeight(height int) int {
	h := height - 7
	if h < 1 {
		h = 1
	}
	return h
}

// scrollGlyph returns the scrollbar cell for visible-window row gi: the thumb
// when gi is inside [thumbStart, thumbEnd), otherwise the track.
func scrollGlyph(t Theme, gi, thumbStart, thumbEnd int) string {
	if gi >= thumbStart && gi < thumbEnd {
		return lipgloss.NewStyle().Foreground(t.BrandLight).Render(ScrollbarThumb)
	}
	return lipgloss.NewStyle().Foreground(t.BorderColor).Render(ScrollbarTrack)
}

// paletteRow renders one palette action as a single line that never wraps: a
// 2-cell marker, the action label, then the dimmed detail after a 2-space gap,
// all truncated to rowW. The cursor row is drawn as the selection band. Keeping
// each action to exactly one line (vs. the old full-width word-wrap) is what
// turns the palette from a wall of bands into a scannable list.
func paletteRow(t Theme, a paletteAction, selected bool, rowW int) string {
	if selected {
		text := a.label
		if a.detail != "" {
			text += "  " + a.detail
		}
		return selectedRow(t, "▶ "+truncateCells(text, rowW-2), rowW)
	}
	avail := rowW - 2 // the 2-cell marker
	label := truncateCells(a.label, avail)
	line := "  " + t.StatusModel.Render(label)
	if rem := avail - lipgloss.Width(label) - 2; a.detail != "" && rem > 0 {
		dim := lipgloss.NewStyle().Foreground(t.FgFaint)
		line += "  " + dim.Render(truncateCells(a.detail, rem))
	}
	return lipgloss.NewStyle().Width(rowW).Render(line)
}

// renderPalette draws the command palette overlay (G5): a filter input above a
// fuzzy-narrowed, single-line list of every action. visible indexes into
// actions; cursor is the position within visible. The list is windowed around
// the cursor so ↑/↓ scroll, with a scrollbar on the right when it overflows.
func renderPalette(t Theme, actions []paletteAction, visible []int, cursor int, filter string, width, height int) string {
	rowW := width - 8
	if rowW < 20 {
		rowW = 20
	}
	var b strings.Builder
	b.WriteString(filterLine(t, filter) + "\n\n")
	if len(visible) == 0 {
		b.WriteString(t.Hint.Render("(no actions match the filter)"))
		header := fmt.Sprintf("%d actions · type to filter · ⏎ run · esc cancel", len(actions))
		return wrapPane(t, "palette", header, b.String(), width, height)
	}

	view := listBodyHeight(height)
	start, end := listWindow(cursor, len(visible), view)
	scroll := len(visible) > view
	if scroll {
		rowW-- // reserve the rightmost cell for the scrollbar glyph
	}
	var thumbStart, thumbEnd int
	if scroll {
		pct := 0.0
		if denom := len(visible) - (end - start); denom > 0 {
			pct = float64(start) / float64(denom)
		}
		thumbStart, thumbEnd = scrollbarThumb(end-start, len(visible), pct)
	}

	for vi := start; vi < end; vi++ {
		row := paletteRow(t, actions[visible[vi]], vi == cursor, rowW)
		if scroll {
			row += scrollGlyph(t, vi-start, thumbStart, thumbEnd)
		}
		b.WriteString(row + "\n")
	}
	header := fmt.Sprintf("%d actions · type to filter · ⏎ run · esc cancel", len(actions))
	if scroll {
		header = fmt.Sprintf("%d actions · %d–%d · type to filter · ⏎ run · esc cancel", len(actions), start+1, end)
	}
	return wrapPane(t, "palette", header, b.String(), width, height)
}

// helpBody returns the structured help body for the overlay (G7): the command
// list (from the shared registry, so it can't drift from the / menu) followed
// by a static keybinding table. It is a plain string so renderHelp can scroll
// it by line offset.
func helpBody(commandRows []slashCmd) string {
	var b strings.Builder
	b.WriteString("commands\n")
	for _, c := range commandRows {
		name := "/" + c.Name
		tag := ""
		switch c.Kind {
		case customCmd:
			tag = " (custom)"
		case skillCmd:
			tag = " (skill)"
		}
		b.WriteString(fmt.Sprintf("  %-18s %s%s\n", name, c.Summary, tag))
	}
	b.WriteString("\nkeys\n")
	for _, k := range keybindingRows() {
		b.WriteString(fmt.Sprintf("  %-12s %s\n", k[0], k[1]))
	}
	return strings.TrimRight(b.String(), "\n")
}

// keybindingRows is the static keybinding table shown in the help overlay
// (G7). Kept as a literal table (not derived from the dispatcher) so it reads
// as documentation; the command list above it IS generated so the two halves
// each have a single source of truth.
func keybindingRows() [][2]string {
	return [][2]string{
		{"⏎", "send prompt (accept completion when the / menu is open)"},
		{"⇧⏎ / ⌥⏎", "insert a newline in the input"},
		{"↑ / ↓", "recall prompt history (insert) · scroll (normal)"},
		{"/", "open the command + skill menu"},
		{"@", "open the file-mention menu"},
		{"^P", "open the command palette"},
		{"?", "open this help (normal mode)"},
		{"^G", "open this help (any mode)"},
		{"esc", "leave insert mode · close a menu / overlay"},
		{"^R", "toggle the most recent thinking block"},
		{"^T", "toggle all thinking blocks"},
		{"^C", "cancel the current run (or quit if idle)"},
		{"^D", "quit"},
		{"j / k", "scroll (normal) · move (overlay)"},
		{"v", "enter visual line-select"},
		{"y", "yank the last assistant text"},
		{"p / P", "open the pager / external pager"},
	}
}

// renderHelp draws the scrollable help overlay (G7). offset is the first
// visible body line (j/k scroll); the body comes from helpBody so the command
// list cannot drift from the / menu.
func renderHelp(t Theme, commandRows []slashCmd, offset, width, height int) string {
	lines := strings.Split(helpBody(commandRows), "\n")
	// Keep every row to one line: truncate to the pane interior (border 2 +
	// horizontal padding 4) so wrapPane can't word-wrap long summaries into the
	// multi-line wall the help used to show.
	interiorW := width - 6
	if interiorW < 10 {
		interiorW = 10
	}
	for i := range lines {
		lines[i] = truncateCells(lines[i], interiorW)
	}
	// Interior height: pane eats border (2) + padding (2) + the title row (1).
	view := height - 5
	if view < 3 {
		view = 3
	}
	if offset < 0 {
		offset = 0
	}
	if offset > len(lines)-view {
		offset = len(lines) - view
	}
	if offset < 0 {
		offset = 0
	}
	end := offset + view
	if end > len(lines) {
		end = len(lines)
	}
	body := strings.Join(lines[offset:end], "\n")
	header := fmt.Sprintf("%d lines · j/k scroll · esc/q close", len(lines))
	return wrapPane(t, "help", header, body, width, height)
}

// selectedRow renders a picker's focused row as a filled selection band:
// brandDeep background with onAccent foreground, padded to width so the
// fill spans the row. When fills are disabled (transparent mode or a
// non-truecolor terminal) it degrades to a bold brandDeep foreground with
// no background, so the ▶ marker in the text still carries the selection.
func selectedRow(t Theme, text string, width int) string {
	style := lipgloss.NewStyle().Width(width)
	if t.Transparent() || !t.Truecolor() {
		return style.Foreground(t.BrandDeep).Bold(true).Render(text)
	}
	return style.
		Background(t.BrandDeep).
		Foreground(t.OnAccent).
		Bold(true).
		Render(text)
}

// wrapPane decorates an overlay body as a raised in-slot pane: a bgRaised
// surface with a rounded border in the border token, Padding(1,2), and a
// gradient "/// <title>" accent header above the hint line. The body is
// padded to fill the available height so the panel background reads as a
// solid slab (degrades to fg-only when fills are disabled).
func wrapPane(t Theme, title, header, body string, width, height int) string {
	gradHead := ApplyBoldForegroundGrad(lipgloss.NewStyle(), "/// "+title, t.BrandDeep, t.BrandLight)
	titleBar := gradHead + "  " + t.Hint.Render(header)

	// Account for border (2) + vertical padding (2) so the body fills the
	// pane's interior without overflowing the allotted height.
	pad := height - 6 - strings.Count(body, "\n")
	if pad > 0 {
		body += strings.Repeat("\n", pad)
	}

	content := titleBar + "\n" + body
	pane := t.Panel(TierRaised).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderColor).
		Padding(1, 2).
		Width(width - 2)
	return pane.Render(content)
}
