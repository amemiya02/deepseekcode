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
	modeModels               // /models — model picker
	modeSessions             // /sessions — session tree picker
)

// Overlay owns the modal-picker state — which picker is up, where
// the cursor sits, and the picker-specific data rows. App composes
// one of these instead of carrying overlay/overlayCursor/models/
// sessionsRows as four flat fields.
type Overlay struct {
	mode         overlayMode
	cursor       int
	models       []modelOption
	sessionsRows []sessionRow
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
func (o *Overlay) Close() { o.mode = modeChat; o.cursor = 0 }

// OpenTape switches to the /tape view with the cursor at the top.
func (o *Overlay) OpenTape() { o.mode = modeTape; o.cursor = 0 }

// OpenModels switches to the /models picker. Cursor lands on the
// row that matches activeID, falling back to 0 if no match.
func (o *Overlay) OpenModels(activeID string) {
	o.models = availableModels()
	o.mode = modeModels
	o.cursor = 0
	for i, m := range o.models {
		if m.ID == activeID {
			o.cursor = i
			break
		}
	}
}

// OpenSessions switches to the /sessions picker with the supplied rows.
func (o *Overlay) OpenSessions(rows []sessionRow) {
	o.sessionsRows = rows
	o.mode = modeSessions
	o.cursor = 0
}

// MoveDown / MoveUp advance the cursor inside the picker.
func (o *Overlay) MoveDown() { o.cursor++ }
func (o *Overlay) MoveUp() {
	if o.cursor > 0 {
		o.cursor--
	}
}

// Models / SessionsRows return the picker-specific row slices for
// rendering. Returned slices are read-only.
func (o *Overlay) Models() []modelOption      { return o.models }
func (o *Overlay) SessionsRows() []sessionRow { return o.sessionsRows }

// SelectedModelID returns the model id under the cursor, or "" when
// the cursor is out of range.
func (o *Overlay) SelectedModelID() string {
	if o.cursor >= 0 && o.cursor < len(o.models) {
		return o.models[o.cursor].ID
	}
	return ""
}

// SelectedSessionID returns the session id under the cursor, or ""
// when the cursor is out of range.
func (o *Overlay) SelectedSessionID() string {
	if o.cursor >= 0 && o.cursor < len(o.sessionsRows) {
		return o.sessionsRows[o.cursor].Sess.ID
	}
	return ""
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

// renderModelsPicker draws the /models picker overlay.
func renderModelsPicker(t Theme, models []modelOption, cursor int, activeID string, width, height int) string {
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= len(models) {
		cursor = len(models) - 1
	}
	rowW := width - 8 // inside border (2) + padding (4) + a small margin
	if rowW < 20 {
		rowW = 20
	}
	var b strings.Builder
	for i, m := range models {
		active := " "
		if m.ID == activeID {
			active = "*"
		}
		if i == cursor {
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
	header := fmt.Sprintf("%d models · ⏎ to switch · esc to cancel", len(models))
	return wrapPane(t, "/models", header, b.String(), width, height)
}

// renderSessionsPicker draws the /sessions picker overlay.
func renderSessionsPicker(t Theme, rows []sessionRow, cursor int, activeID string, width, height int) string {
	if len(rows) == 0 {
		body := t.Hint.Render("(no sessions in this project yet)")
		return wrapPane(t, "/sessions", "0 sessions", body, width, height)
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= len(rows) {
		cursor = len(rows) - 1
	}
	rowW := width - 8 // inside border (2) + padding (4) + a small margin
	if rowW < 20 {
		rowW = 20
	}
	var b strings.Builder
	for i, r := range rows {
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
		if i == cursor {
			// Selected row: brandDeep bg-fill with onAccent fg, plus ▶ marker.
			text := fmt.Sprintf("▶ %s %s%s  %s  %s", active, indent, shortID, when, summary)
			b.WriteString(selectedRow(t, text, rowW) + "\n")
		} else {
			marker := " "
			if r.Sess.ID == activeID {
				marker = t.StatusGood.Render("*")
			}
			line := fmt.Sprintf("  %s %s%s  %s  %s",
				marker, indent, t.StatusModel.Render(shortID), when, summary)
			b.WriteString(line + "\n")
		}
	}
	header := fmt.Sprintf("%d sessions · ⏎ to switch · esc to cancel", len(rows))
	return wrapPane(t, "/sessions", header, b.String(), width, height)
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
