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
	modeThemes               // /theme — theme picker (filterable, live preview)
	modeMCP                  // /mcp — MCP server status overlay
	modeLSP                  // /lsp — LSP server status overlay
	modePermissions          // /permissions — effective policy overlay
	modeEffort               // /effort — reasoning-effort picker (filterable)
	modeQuitConfirm          // quit-confirm y/n dialog
	modeFilePicker           // file-picker dialog (filterable)
)

// Named help-tab indices. They index a switch inside renderHelp, not an
// overlayMode, so they are plain untyped int consts.
const (
	helpTabGeneral  = 0
	helpTabCommands = 1
	helpTabCustom   = 2
	helpTabCount    = 3
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
	helpTab      int // active help tab (0..helpTabCount-1); only meaningful in modeHelp
	models       []modelOption
	sessionsRows []sessionRow
	palette      []paletteAction
	themes       []themeOption
	mcpServers   []McpServerRow
	lspServers   []LspServerRow
	permRows     []PermissionRow
	efforts       []string // reasoning-effort rows: low, medium, high, max
	filePickerPaths []string // file paths for the file-picker dialog
	filter        filterableList
}

// McpServerRow is one row in the /mcp status overlay.
type McpServerRow struct {
	Name         string
	State        string // "connected", "degraded", "failed", etc.
	ToolCount    int
	Tools        []string
	BackoffUntil string // human-readable, or empty
	LastError    string
}

// LspServerRow is one row in the /lsp status overlay.
type LspServerRow struct {
	Name            string
	Command         string
	Connected       bool
	DiagnosticCount int
	LastError       string
}

// PermissionRow is one row in the /permissions status overlay.
// Key/Value are displayed as "Key .... Value" pairs.
type PermissionRow struct {
	Key   string
	Value string
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

// OpenModelsRows seeds the picker from registry-supplied rows instead of the
// hard-coded defaults.  Callers that build rows from the model registry
// (provider grouping, availability flags) use this entry point.
func (o *Overlay) OpenModelsRows(activeID string, rows []modelOption) {
	o.models = rows
	o.mode = modeModels
	o.cursor = 0
	o.filter = filterableList{} // reset filter state
	labels := make([]string, len(rows))
	for i, m := range rows {
		labels[i] = m.Short + " " + m.Note
	}
	o.filter.SetRows(labels)
	for i, m := range rows {
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
func (o *Overlay) OpenHelp() { o.mode = modeHelp; o.cursor = 0; o.helpTab = 0 }

// HelpTab returns the active help tab index (0..helpTabCount-1).
func (o *Overlay) HelpTab() int { return o.helpTab }

// SetHelpTab sets the active help tab, clamped to [0, helpTabCount-1], and
// resets the scroll offset (cursor) to 0. A no-op-safe clamp: negative -> 0,
// >= helpTabCount -> helpTabCount-1.
func (o *Overlay) SetHelpTab(i int) {
	if i < 0 {
		i = 0
	} else if i >= helpTabCount {
		i = helpTabCount - 1
	}
	o.helpTab = i
	o.cursor = 0
}

// NextHelpTab / PrevHelpTab cycle the active help tab with wrap-around
// (Next from the last tab -> first; Prev from the first -> last) and reset the
// scroll offset (cursor) to 0.
func (o *Overlay) NextHelpTab() { o.SetHelpTab((o.helpTab + 1) % helpTabCount) }
func (o *Overlay) PrevHelpTab() { o.SetHelpTab((o.helpTab - 1 + helpTabCount) % helpTabCount) }

// Filterable reports whether the active overlay routes typing into the shared
// filter (the pickers and the palette) rather than treating letters as nav
// keys. modeTape and modeHelp are not filterable.
func (o *Overlay) Filterable() bool {
	switch o.mode {
	case modeModels, modeSessions, modePalette, modeThemes, modeMCP, modeLSP, modeEffort, modeFilePicker:
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

// SelectedModelOption returns the full modelOption under the cursor (mapped
// through the filter), or the zero value and false when nothing matches.
func (o *Overlay) SelectedModelOption() (modelOption, bool) {
	if i := o.filter.Selected(); i >= 0 && i < len(o.models) {
		return o.models[i], true
	}
	return modelOption{}, false
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

// OpenThemes switches to the /theme picker. The filterableList is seeded with
// "Label  Desc" per row; the cursor lands on the row that matches activeID,
// falling back to 0 if no match.
func (o *Overlay) OpenThemes(activeID string) {
	o.themes = availableThemes()
	o.mode = modeThemes
	o.cursor = 0
	labels := make([]string, len(o.themes))
	for i, th := range o.themes {
		labels[i] = th.Label + "  " + th.Desc
	}
	o.filter.SetRows(labels)
	for i, th := range o.themes {
		if th.ID == activeID {
			o.cursorTo(i)
			break
		}
	}
}

// SelectedThemeID returns the theme id under the cursor (mapped through the
// filter), or "" when nothing matches.
func (o *Overlay) SelectedThemeID() string {
	if i := o.filter.Selected(); i >= 0 && i < len(o.themes) {
		return o.themes[i].ID
	}
	return ""
}

// Themes returns the picker rows (read-only).
func (o *Overlay) Themes() []themeOption { return o.themes }

// OpenEffort switches to the reasoning-effort picker. The filterableList is
// seeded with the four effort levels; the cursor lands on the row that matches
// current, falling back to 0 if no match.
func (o *Overlay) OpenEffort(current string) {
	o.efforts = []string{"low", "medium", "high", "max"}
	o.mode = modeEffort
	o.cursor = 0
	labels := make([]string, len(o.efforts))
	copy(labels, o.efforts)
	o.filter.SetRows(labels)
	for i, e := range o.efforts {
		if e == current {
			o.cursorTo(i)
			break
		}
	}
}

// SelectedEffort returns the effort level under the cursor (mapped through the
// filter), or "" when nothing matches.
func (o *Overlay) SelectedEffort() string {
	if i := o.filter.Selected(); i >= 0 && i < len(o.efforts) {
		return o.efforts[i]
	}
	return ""
}

// Efforts returns the effort picker rows (read-only).
func (o *Overlay) Efforts() []string { return o.efforts }

// OpenFilePicker switches to the file-picker dialog. The filterableList is
// seeded with the supplied repo-relative paths; the cursor lands on the first
// row.
func (o *Overlay) OpenFilePicker(paths []string) {
	o.filePickerPaths = paths
	o.mode = modeFilePicker
	o.cursor = 0
	labels := make([]string, len(paths))
	copy(labels, paths)
	o.filter.SetRows(labels)
}

// SelectedFile returns the file path under the cursor (mapped through the
// filter), or "" when nothing matches.
func (o *Overlay) SelectedFile() string {
	if i := o.filter.Selected(); i >= 0 && i < len(o.filePickerPaths) {
		return o.filePickerPaths[i]
	}
	return ""
}

// FilePickerPaths returns the file-picker rows (read-only).
func (o *Overlay) FilePickerPaths() []string { return o.filePickerPaths }

// OpenMCP switches to the /mcp status overlay.
func (o *Overlay) OpenMCP(rows []McpServerRow) {
	o.mcpServers = rows
	o.mode = modeMCP
	o.cursor = 0
	labels := make([]string, len(rows))
	for i, r := range rows {
		labels[i] = r.Name + " " + r.State
	}
	o.filter.SetRows(labels)
}

// MCPServers returns the MCP server rows (read-only).
func (o *Overlay) MCPServers() []McpServerRow { return o.mcpServers }

// OpenLSP switches to the /lsp status overlay.
func (o *Overlay) OpenLSP(rows []LspServerRow) {
	o.lspServers = rows
	o.mode = modeLSP
	o.cursor = 0
	labels := make([]string, len(rows))
	for i, r := range rows {
		conn := "disconnected"
		if r.Connected {
			conn = "connected"
		}
		labels[i] = r.Name + " " + conn
	}
	o.filter.SetRows(labels)
}

// LSPServers returns the LSP server rows (read-only).
func (o *Overlay) LSPServers() []LspServerRow { return o.lspServers }

// OpenPermissions switches to the /permissions status overlay.
func (o *Overlay) OpenPermissions(rows []PermissionRow) {
	o.permRows = rows
	o.mode = modePermissions
	o.cursor = 0
}

// Permissions returns the permission rows (read-only).
func (o *Overlay) Permissions() []PermissionRow { return o.permRows }

// quitDecision is the outcome of the quit-confirm dialog.
type quitDecision int

const (
	quitNone quitDecision = iota
	quitConfirmed
	quitCancel
)

// OpenQuitConfirm switches to the quit-confirm dialog (y/n prompt).
func (o *Overlay) OpenQuitConfirm() {
	o.mode = modeQuitConfirm
	o.cursor = 0
}

// QuitConfirmResolve processes a key press in the quit-confirm dialog.
// "y"/"Y" confirms quit, "n"/"N"/"esc" cancels, anything else is ignored.
func (o *Overlay) QuitConfirmResolve(key string) quitDecision {
	switch key {
	case "y", "Y":
		o.Close()
		return quitConfirmed
	case "n", "N", "esc":
		o.Close()
		return quitCancel
	default:
		return quitNone
	}
}

// renderQuitConfirm draws the quit-confirm dialog as a small centered y/n card.
func renderQuitConfirm(t Theme, width, height int) string {
	body := t.Hint.Render("y") + "  quit  " + t.Hint.Render("n") + "  cancel"
	return wrapPane(t, "quit", "confirm?", body, width, height)
}

// modelOption is one row in the /models picker.
type modelOption struct {
	ID        string
	Short     string
	Note      string // e.g. pricing summary
	Provider  string // registry provider name (e.g. "deepseek")
	Available bool   // false when the model cannot be reached right now
}

// availableModels returns the picker rows for /models. Pricing is
// hard-coded here (mirrors internal/llm/cache_metrics.go) so the
// picker is informative even offline. Official V4 models appear first;
// legacy aliases are listed below with the retirement date.
func availableModels() []modelOption {
	return []modelOption{
		{ID: "deepseek-v4-flash", Short: "flash", Note: "1M ctx · ¥1/¥0.02 in · ¥2 out · default", Provider: "deepseek", Available: true},
		{ID: "deepseek-v4-pro", Short: "pro", Note: "1M ctx · ¥3/¥0.025 in · ¥6 out · stronger", Provider: "deepseek", Available: true},
		{ID: "deepseek-chat", Short: "chat", Note: "legacy until 2026-07-24 · alias → flash", Provider: "deepseek", Available: true},
		{ID: "deepseek-reasoner", Short: "reasoner", Note: "legacy until 2026-07-24 · alias → flash", Provider: "deepseek", Available: true},
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
	rowW := width - 4 // 2-cell gutter each side (wrapPane indents the body by 2)
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
//
// When rows carry a Provider field, consecutive visible rows from different
// providers are separated by a dimmed provider header line.  Unavailable rows
// (Available == false) render with dimmed text and an "(unavailable)" suffix.
func renderModelsPicker(t Theme, models []modelOption, visible []int, cursor int, filter, activeID string, width, height int) string {
	rowW := width - 4 // 2-cell gutter each side (wrapPane indents the body by 2)
	if rowW < 20 {
		rowW = 20
	}
	var b strings.Builder
	b.WriteString(filterLine(t, filter) + "\n\n")
	if len(visible) == 0 {
		b.WriteString(t.Hint.Render("(no models match the filter)"))
	}
	prevProvider := ""
	for vi, idx := range visible {
		m := models[idx]

		// Emit a provider header when the group changes.
		if m.Provider != "" && m.Provider != prevProvider {
			b.WriteString(t.Hint.Render("  "+m.Provider) + "\n")
			prevProvider = m.Provider
		}

		active := " "
		if m.ID == activeID {
			active = "*"
		}
		note := m.Note
		if !m.Available {
			note = note + " (unavailable)"
		}
		if vi == cursor {
			// Selected row: brandDeep bg-fill with onAccent fg spanning the
			// row, in addition to the ▶ marker. Plain text inside the filled
			// style so the foreground stays legible on the accent band.
			text := fmt.Sprintf("▶ %s %s  %s", active, m.Short, note)
			b.WriteString(selectedRow(t, truncateCells(text, rowW), rowW) + "\n")
		} else {
			marker := " "
			if m.ID == activeID {
				marker = t.StatusGood.Render("*")
			}
			var line string
			if !m.Available {
				// Dim the entire row for unavailable models.
				line = fmt.Sprintf("  %s %s  %s", marker, t.Hint.Render(m.Short), t.Hint.Render(note))
			} else {
				line = fmt.Sprintf("  %s %s  %s", marker, t.StatusModel.Render(m.Short), t.Hint.Render(note))
			}
			b.WriteString(line + "\n")
		}
	}
	header := fmt.Sprintf("%d models · type to filter · ⏎ switch · esc cancel", len(models))
	return wrapPane(t, "/models", header, b.String(), width, height)
}

// renderThemesPicker draws the /theme picker overlay. Each row shows a color
// swatch built from the row's own theme colors (brandDeep, accentFlash,
// accentPro), then the Label and dim Desc. The active theme is marked with *,
// the cursor row uses selectedRow. Mirrors renderModelsPicker's structure.
func renderThemesPicker(t Theme, rows []themeOption, visible []int, cursor int, filter, activeID string, width, height int) string {
	rowW := width - 4
	if rowW < 20 {
		rowW = 20
	}
	var b strings.Builder
	b.WriteString(filterLine(t, filter) + "\n\n")
	if len(visible) == 0 {
		b.WriteString(t.Hint.Render("(no themes match the filter)"))
	}
	// Swatch: three colored blocks. Fixed 6-cell display width.
	swatchW := 6
	for vi, idx := range visible {
		th := themeByID(rows[idx].ID)
		swatch := lipgloss.NewStyle().Foreground(th.BrandDeep).Render("█") +
			lipgloss.NewStyle().Foreground(th.AccentFlash).Render("█") +
			lipgloss.NewStyle().Foreground(th.AccentPro).Render("█")
		active := " "
		if rows[idx].ID == activeID {
			active = "*"
		}
		if vi == cursor {
			// Selected row: swatch + label + desc, all inside the selection band.
			text := fmt.Sprintf("▶ %s %s  %s  %s", active, swatch, rows[idx].Label, rows[idx].Desc)
			b.WriteString(selectedRow(t, truncateCells(text, rowW), rowW) + "\n")
		} else {
			marker := " "
			if rows[idx].ID == activeID {
				marker = t.StatusGood.Render("*")
			}
			label := t.StatusModel.Render(rows[idx].Label)
			// Truncate only the label+desc tail so the swatch stays fixed.
			remain := rowW - 2 - 1 - swatchW - 4 // "  " + marker + " " + swatch + "  "
			if remain < 0 {
				remain = 0
			}
			descRemain := remain - lipgloss.Width(label)
			if descRemain < 0 {
				descRemain = 0
			}
			line := fmt.Sprintf("  %s %s  %s  %s", marker, swatch, label, truncateCells(rows[idx].Desc, descRemain))
			b.WriteString(line + "\n")
		}
	}
	header := fmt.Sprintf("%d themes · type to filter · ⏎ apply · esc cancel", len(rows))
	return wrapPane(t, "/theme", header, b.String(), width, height)
}

// renderEffortPicker draws the reasoning-effort picker overlay. Each row is
// simply the effort level name. The active effort is marked with *, the cursor
// row uses selectedRow. Mirrors renderModelsPicker's structure.
func renderEffortPicker(t Theme, efforts []string, visible []int, cursor int, filter, activeEffort string, width, height int) string {
	rowW := width - 4
	if rowW < 20 {
		rowW = 20
	}
	var b strings.Builder
	b.WriteString(filterLine(t, filter) + "\n\n")
	if len(visible) == 0 {
		b.WriteString(t.Hint.Render("(no effort levels match the filter)"))
	}
	for vi, idx := range visible {
		effort := efforts[idx]
		active := " "
		if effort == activeEffort {
			active = "*"
		}
		if vi == cursor {
			text := fmt.Sprintf("▶ %s %s", active, effort)
			b.WriteString(selectedRow(t, truncateCells(text, rowW), rowW) + "\n")
		} else {
			marker := " "
			if effort == activeEffort {
				marker = t.StatusGood.Render("*")
			}
			line := fmt.Sprintf("  %s %s", marker, t.StatusModel.Render(effort))
			b.WriteString(line + "\n")
		}
	}
	header := fmt.Sprintf("%d levels · type to filter · ⏎ apply · esc cancel", len(efforts))
	return wrapPane(t, "/effort", header, b.String(), width, height)
}

// renderFilePicker draws the file-picker dialog overlay. Each row shows a
// repo-relative file path. Mirrors renderEffortPicker's structure.
func renderFilePicker(t Theme, paths []string, visible []int, cursor int, filter string, width, height int) string {
	rowW := width - 4
	if rowW < 20 {
		rowW = 20
	}
	var b strings.Builder
	b.WriteString(filterLine(t, filter) + "\n\n")
	if len(visible) == 0 {
		b.WriteString(t.Hint.Render("(no files match the filter)"))
	}
	for vi, idx := range visible {
		path := paths[idx]
		if vi == cursor {
			text := fmt.Sprintf("▶ %s", path)
			b.WriteString(selectedRow(t, truncateCells(text, rowW), rowW) + "\n")
		} else {
			line := fmt.Sprintf("  %s", t.StatusModel.Render(path))
			b.WriteString(truncateCells(line, rowW) + "\n")
		}
	}
	header := fmt.Sprintf("%d files · type to filter · ⏎ select · esc cancel", len(paths))
	return wrapPane(t, "file", header, b.String(), width, height)
}

// renderSessionsPicker draws the /sessions picker overlay. visible is the
// fuzzy-narrowed row order (indices into rows); cursor is the position within
// visible. A filter input rides above the rows (G6).
func renderSessionsPicker(t Theme, rows []sessionRow, visible []int, cursor int, filter, activeID string, width, height int) string {
	if len(rows) == 0 {
		body := t.Hint.Render("(no sessions in this project yet)")
		return wrapPane(t, "/sessions", "0 sessions", body, width, height)
	}
	rowW := width - 4 // 2-cell gutter each side (wrapPane indents the body by 2)
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
// surface `height` tall: the surface chrome (top margin 1 + title 1 + rule 1 +
// blank 1) and the filter input plus its blank separator (2) come off the top.
func listBodyHeight(height int) int {
	h := height - 6
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
	rowW := width - 4 // 2-cell gutter each side (wrapPane indents the body by 2)
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

// helpTabTitles holds the display names for each help tab, indexed by the
// helpTab* consts.
var helpTabTitles = [helpTabCount]string{"General", "Commands", "Custom commands"}

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

// --- tabbed help body builders (Task 4102) -----------------------------------

// helpIntroText returns the one-sentence description shown atop the General tab.
// Plain unstyled string (no trailing newline), wrapped by the caller.
func helpIntroText() string {
	return "deepseekcode reads your codebase, proposes edits you approve, and runs tools — all from your terminal, powered by DeepSeek models."
}

// twoColMinWidth is the minimum pane interior width at which columnize lays
// cells out in two columns. Below this it falls back to a single column.
const twoColMinWidth = 80

// columnize lays cells into 2 columns when width >= twoColMinWidth, else 1
// column. Fill is COLUMN-MAJOR: the left column gets the first ceil(n/2) cells
// top-to-bottom, the right column the rest. Each returned element is one row
// string; each cell is truncated to the per-column width (truncateCells). A
// 2-space gutter separates columns. Returns nil for an empty input.
func columnize(cells []string, width int) []string {
	if len(cells) == 0 {
		return nil
	}
	if width < twoColMinWidth {
		out := make([]string, len(cells))
		for i, c := range cells {
			out[i] = truncateCells(c, width)
		}
		return out
	}
	half := (len(cells) + 1) / 2
	colW := (width - 2) / 2 // 2 = gutter
	if colW < 1 {
		colW = 1
	}
	out := make([]string, half)
	for i := 0; i < half; i++ {
		left := truncateCells(cells[i], colW)
		right := ""
		if i+half < len(cells) {
			right = truncateCells(cells[i+half], colW)
		}
		// Pad left to colW display cells so the gutter stays fixed.
		leftPad := lipgloss.NewStyle().Width(colW).Render(left)
		out[i] = leftPad + "  " + right
	}
	return out
}

// helpGeneralBody returns the General tab body: the intro sentence wrapped to
// width, a blank line, a "shortcuts" header, then the keybindingRows() table
// laid out via columnize at width. width is the pane interior width.
func helpGeneralBody(t Theme, width int) string {
	intro := lipgloss.NewStyle().Width(width).Render(helpIntroText())
	cells := make([]string, len(keybindingRows()))
	for i, k := range keybindingRows() {
		cells[i] = fmt.Sprintf("%-10s %s", k[0], k[1])
	}
	rows := columnize(cells, width)
	return strings.TrimRight(intro+"\n\n"+"shortcuts"+"\n"+strings.Join(rows, "\n"), "\n")
}

// helpCommandsBody returns the Commands tab body: a "commands" header then one
// line per built-in command (rows with Kind == builtinCmd), formatted
// "  /name  summary". Rows are taken from commandRows in their given order.
func helpCommandsBody(commandRows []slashCmd) string {
	var b strings.Builder
	b.WriteString("commands\n")
	for _, c := range commandRows {
		if c.Kind != builtinCmd {
			continue
		}
		fmt.Fprintf(&b, "  %-18s %s\n", "/"+c.Name, c.Summary)
	}
	return strings.TrimRight(b.String(), "\n")
}

// helpCustomBody returns the Custom-commands tab body: a header then one line
// per row with Kind == customCmd or skillCmd, formatted "  /name  summary (custom)"
// or "(skill)". When no such rows exist, the body is the header + a single
// dimmed-able empty-state line "  (no custom commands or skills found)".
func helpCustomBody(commandRows []slashCmd) string {
	var b strings.Builder
	b.WriteString("custom commands & skills\n")
	count := 0
	for _, c := range commandRows {
		if c.Kind != customCmd && c.Kind != skillCmd {
			continue
		}
		tag := " (custom)"
		if c.Kind == skillCmd {
			tag = " (skill)"
		}
		fmt.Fprintf(&b, "  %-18s %s%s\n", "/"+c.Name, c.Summary, tag)
		count++
	}
	if count == 0 {
		b.WriteString("  (no custom commands or skills found)\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderHelpTabBar renders the three tab titles on ONE line: active tab as a
// filled chip (SelBg/SelFg, bold, padded 0,1), inactive as dim (t.Hint, padded
// 0,1), separated by a single space. Degrades to bold BrandLight fg (no bg) for
// the active tab when t.Transparent() || !t.Truecolor(). The whole line is
// truncated to width. active is a helpTab* index.
func renderHelpTabBar(t Theme, active, width int) string {
	var parts []string
	for i, title := range helpTabTitles {
		style := lipgloss.NewStyle().Padding(0, 1)
		if i == active {
			if t.Transparent() || !t.Truecolor() {
				style = style.Foreground(t.BrandLight).Bold(true)
			} else {
				style = style.Background(t.SelBg).Foreground(t.SelFg).Bold(true)
			}
		} else {
			style = style.Foreground(t.FgFaint)
		}
		parts = append(parts, style.Render(title))
	}
	return truncateCells(strings.Join(parts, " "), width)
}

// renderHelp draws the tabbed help overlay. tab selects the body
// (helpTabGeneral/Commands/Custom); offset is the first visible BODY line of
// the active tab (j/k scroll). Tab bar + a blank line are fixed chrome above
// the scrolled body; the scroll viewport is height-6. Always returns exactly
// height rows × width cells via wrapPane.
func renderHelp(t Theme, commandRows []slashCmd, tab, offset, width, height int) string {
	interiorW := width - 6
	if interiorW < 10 {
		interiorW = 10
	}

	// Select body by tab index.
	var body string
	switch tab {
	case helpTabCommands:
		body = helpCommandsBody(commandRows)
	case helpTabCustom:
		body = helpCustomBody(commandRows)
	default:
		body = helpGeneralBody(t, interiorW)
	}

	lines := strings.Split(body, "\n")
	for i := range lines {
		lines[i] = truncateCells(lines[i], interiorW)
	}

	// Tab bar + blank are fixed chrome; the rest is the scrollable body.
	view := height - 6
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
	tabBar := renderHelpTabBar(t, tab, interiorW)
	bodyOut := tabBar + "\n\n" + strings.Join(lines[offset:end], "\n")

	title := "General"
	if tab >= 0 && tab < helpTabCount {
		title = helpTabTitles[tab]
	}
	header := fmt.Sprintf("%s · %d lines · tab/←→ switch · j/k scroll", title, len(lines))
	return wrapPane(t, "help", header, bodyOut, width, height)
}

// renderMCP draws the /mcp status overlay. Each row shows server name,
// lifecycle state, tool count, and last error if any.
func renderMCP(t Theme, rows []McpServerRow, visible []int, cursor int, filter string, width, height int) string {
	if len(rows) == 0 {
		body := t.Hint.Render("(no MCP servers configured)")
		return wrapPane(t, "/mcp", "0 servers", body, width, height)
	}
	rowW := width - 4
	if rowW < 20 {
		rowW = 20
	}
	var b strings.Builder
	b.WriteString(filterLine(t, filter) + "\n\n")
	if len(visible) == 0 {
		b.WriteString(t.Hint.Render("(no servers match the filter)"))
		header := fmt.Sprintf("%d servers · type to filter · esc cancel", len(rows))
		return wrapPane(t, "/mcp", header, b.String(), width, height)
	}

	for vi, idx := range visible {
		r := rows[idx]
		stateStyle := t.Status
		switch r.State {
		case "connected":
			stateStyle = t.StatusGood
		case "degraded", "failed":
			stateStyle = t.StatusBad
		}
		backoffText := ""
		if r.BackoffUntil != "" {
			backoffText = "  backoff " + r.BackoffUntil
		}
		errText := ""
		if r.LastError != "" {
			errText = "  " + t.StatusBad.Render(truncateCells(r.LastError, 30))
		}
		if vi == cursor {
			text := fmt.Sprintf("▶ %s  %s  %d tools%s%s", r.Name, r.State, r.ToolCount, backoffText, errText)
			b.WriteString(selectedRow(t, truncateCells(text, rowW), rowW) + "\n")
		} else {
			line := fmt.Sprintf("  %s  %s  %s%s%s",
				t.StatusModel.Render(r.Name),
				stateStyle.Render(r.State),
				t.Hint.Render(fmt.Sprintf("%d tools", r.ToolCount)),
				backoffText, errText)
			b.WriteString(truncateCells(line, rowW) + "\n")
		}
	}
	header := fmt.Sprintf("%d servers · type to filter · esc cancel", len(rows))
	return wrapPane(t, "/mcp", header, b.String(), width, height)
}

// renderLSP draws the /lsp status overlay. Each row shows server name,
// command, connection state, diagnostic count, and last error if any.
func renderLSP(t Theme, rows []LspServerRow, visible []int, cursor int, filter string, width, height int) string {
	if len(rows) == 0 {
		body := t.Hint.Render("(no LSP servers detected)")
		return wrapPane(t, "/lsp", "0 servers", body, width, height)
	}
	rowW := width - 4
	if rowW < 20 {
		rowW = 20
	}
	var b strings.Builder
	b.WriteString(filterLine(t, filter) + "\n\n")
	if len(visible) == 0 {
		b.WriteString(t.Hint.Render("(no servers match the filter)"))
		header := fmt.Sprintf("%d servers · type to filter · esc cancel", len(rows))
		return wrapPane(t, "/lsp", header, b.String(), width, height)
	}

	for vi, idx := range visible {
		r := rows[idx]
		connStr := "disconnected"
		connStyle := t.Status
		if r.Connected {
			connStr = "connected"
			connStyle = t.StatusGood
		}
		cmdText := ""
		if r.Command != "" {
			cmdText = "  " + r.Command
		}
		diagText := fmt.Sprintf("  %d diagnostics", r.DiagnosticCount)
		errText := ""
		if r.LastError != "" {
			errText = "  " + t.StatusBad.Render(truncateCells(r.LastError, 30))
		}
		if vi == cursor {
			text := fmt.Sprintf("▶ %s  %s%s%s%s", r.Name, connStr, cmdText, diagText, errText)
			b.WriteString(selectedRow(t, truncateCells(text, rowW), rowW) + "\n")
		} else {
			line := fmt.Sprintf("  %s  %s%s%s%s",
				t.StatusModel.Render(r.Name),
				connStyle.Render(connStr),
				t.Hint.Render(cmdText),
				t.Hint.Render(diagText),
				errText)
			b.WriteString(truncateCells(line, rowW) + "\n")
		}
	}
	header := fmt.Sprintf("%d servers · type to filter · esc cancel", len(rows))
	return wrapPane(t, "/lsp", header, b.String(), width, height)
}

// renderPermissions draws the /permissions status overlay. Each row shows a
// policy key and its current value. This is a non-filterable, non-selectable
// status display — the user reads it and presses esc to close.
func renderPermissions(t Theme, rows []PermissionRow, width, height int) string {
	if len(rows) == 0 {
		body := t.Hint.Render("(no permission policy loaded)")
		return wrapPane(t, "/permissions", "no policy", body, width, height)
	}
	rowW := width - 4
	if rowW < 20 {
		rowW = 20
	}
	var b strings.Builder
	for _, r := range rows {
		keyStr := t.StatusModel.Render(r.Key)
		valStr := r.Value
		// Dot leader between key and value for visual alignment.
		dots := ""
		keyW := lipgloss.Width(keyStr)
		valW := lipgloss.Width(valStr)
		gap := rowW - keyW - valW - 2 // 2 for padding
		if gap > 2 {
			dots = t.Hint.Render(strings.Repeat("·", gap))
		} else {
			dots = "  "
		}
		b.WriteString("  " + keyStr + dots + valStr + "\n")
	}
	header := fmt.Sprintf("%d entries · esc close", len(rows))
	return wrapPane(t, "/permissions", header, b.String(), width, height)
}

// selectedRow renders a picker's focused row as a filled selection band: a
// calm deep-indigo SelBg surface carrying bright SelFg text, padded to width so
// the fill spans the row. The band is intentionally a recessive surface (not
// the bright brand) with high-luminance text on top — a confident, classy
// highlight rather than the vibrating royal-blue-on-near-black slab it replaced.
// When fills are disabled (transparent mode or a non-truecolor terminal) it
// degrades to bold BrandLight foreground with no background, so the ▶ marker in
// the text still carries the selection.
func selectedRow(t Theme, text string, width int) string {
	style := lipgloss.NewStyle().Width(width)
	if t.Transparent() || !t.Truecolor() {
		return style.Foreground(t.BrandLight).Bold(true).Render(text)
	}
	return style.
		Background(t.SelBg).
		Foreground(t.SelFg).
		Bold(true).
		Render(text)
}

// wrapPane renders an overlay as a full-screen, edge-to-edge surface (the
// /models /sessions palette help /tape pickers). The overlay REPLACES the chat
// (renderOverlay returns early from View), so its content is composed entirely
// of our own painted strings — no glamour / viewport output — which means
// ADR-0002's reset-bleed caveat does not apply and we can safely fill the whole
// width×height here. We paint ONE cohesive bgBase surface: a faint top margin, a
// gradient "/// <title>" header with its dim hint, a hairline rule, the body,
// then bgBase padding all the way to the bottom. This replaces the old bgRaised
// rounded card that floated on the terminal's raw black — the "navy slab on a
// black void" the redesign set out to kill. Degrades to fg-only (no fill) when
// backgrounds are disabled (transparent mode / non-truecolor), per ADR-0002.
func wrapPane(t Theme, title, header, body string, width, height int) string {
	const indent = "  "
	surface := t.Panel(TierBase)

	gradHead := ApplyBoldForegroundGrad(lipgloss.NewStyle(), "/// "+title, t.BrandDeep, t.BrandLight)
	titleBar := indent + gradHead + "  " + t.Hint.Render(header)

	ruleW := width - 2*len(indent)
	if ruleW < 0 {
		ruleW = 0
	}
	rule := indent + lipgloss.NewStyle().Foreground(t.BorderColor).Render(strings.Repeat("─", ruleW))

	// A faint top margin, the header, the rule, a blank, then the body. Every
	// body line is shifted into the same `indent` gutter so the filter line,
	// rows, and selection band all left-align under the title (the body's own
	// internal layout — markers, scrollbar — rides inside that gutter).
	lines := []string{"", titleBar, rule, ""}
	for _, bl := range strings.Split(body, "\n") {
		lines = append(lines, indent+bl)
	}

	// Pad (or clamp) to EXACTLY height rows so the surface fills the band the
	// overlay was handed and the View stack stays the right height — every blank
	// row is painted below, so there is no unpainted gap left to leak black.
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}

	// Width pads every line (blanks included) to the full width with the surface
	// background, so the whole rectangle is painted edge to edge.
	return surface.Width(width).Render(strings.Join(lines, "\n"))
}

// overlayFooter renders the bottom hint row of a full-screen overlay on the
// SAME bgBase surface as wrapPane, so the footer is part of the painted slab
// rather than a fg-only line sitting on raw black. Degrades to fg-only when
// fills are disabled.
func overlayFooter(t Theme, text string, width int) string {
	return t.Panel(TierBase).Foreground(t.FgFaint).Width(width).Render("  " + text)
}
