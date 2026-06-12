package tui

import (
	"encoding/json"
	"fmt"
	"image/color"
	"sort"
	"strings"
	"time"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// itemKind enumerates the kinds of items the scrollback renders. New
// kinds added here should pair with a case in chatItem.render().
//
// Mapping to llm.BlockKind (each chatItem represents one rendered
// block; the scrollback builds items incrementally from stream
// events rather than from a whole llm.Message, so there is no
// Message→items conversion path here):
//
//	itemUser           ← user-role TextBlock
//	itemAssistantText  ← assistant-role TextBlock
//	itemReasoning      ← ThinkingBlock
//	itemToolCall       ← ToolUseBlock
//	itemToolResult     ← ToolResultBlock
//
// The remaining itemKinds (Info, StepFinish, Error, Welcome) are
// TUI-only artifacts with no on-the-wire block counterpart.
type itemKind int

const (
	itemUser itemKind = iota + 1
	itemAssistantText
	itemReasoning
	itemToolCall
	itemToolResult
	itemHookFired
	itemRepair
	itemInfo
	itemStepFinish
	itemError
	itemWelcome
)

// chatItem is one rendered entry in the scrollback. We keep raw fields
// rather than pre-rendered strings so theme switching or width changes
// can re-render without losing data.
type chatItem struct {
	kind itemKind

	// Common
	timestamp time.Time

	// User / Assistant / Info / Error
	text string

	// Reasoning
	reasoning string
	folded    bool
	tokens    int // rough estimate for collapsed label
	duration  time.Duration

	// ToolCall / ToolResult
	toolCallID string
	tool       string
	args       string
	result     tools.Result
	expanded   bool // true when user has expanded a truncated tool result

	// HookFired
	hookDecision string
	hookReason   string

	// Repair
	repairKind    string
	repairTool    string
	repairMessage string

	// StepFinish
	stopReason string
	usage      llm.Usage
	model      string
}

func (i chatItem) renderKey(width int, theme string) string {
	switch i.kind {
	case itemToolResult:
		return fmt.Sprintf(
			"tool-result:%s:%s:%s:%s:%t:%t:%d:%s:%s",
			i.toolCallID,
			i.tool,
			i.args,
			theme,
			i.expanded,
			i.result.IsError,
			width,
			i.duration.Round(time.Millisecond).String(),
			i.result.Content,
		)
	case itemToolCall:
		return fmt.Sprintf("tool-call:%s:%s:%s:%d:%s", i.toolCallID, i.tool, theme, width, i.args)
	default:
		return ""
	}
}

// gutterLine prefixes one logical line with the 2-cell gutter. Used for item
// kinds that carry no left bar (assistant prose lives airy on the canvas, but
// still shares the universal indent).
func gutterLine(t Theme, s string) string {
	return t.Gutter() + s
}

// barLine prefixes one logical line with the 2-cell gutter followed by a
// role-colored left bar glyph rendered through c, then a single space. This is
// the universal opener for every barred item kind (user / tool / reasoning) so
// the gutter + bar geometry is defined in exactly one place.
func barLine(t Theme, c color.Color, s string) string {
	return t.Gutter() + t.LeftBar(c).Render(LeftBarGlyph()) + " " + s
}

// toolAccent picks the tool bar/accent color for an item: accentPro when the
// item was produced by the pro model, accentFlash otherwise. itemToolCall /
// itemToolResult only carry a model when the scrollback populates it; the
// flash default is correct for the common (unset) case.
func toolAccent(t Theme, model string) color.Color {
	if model == "deepseek-v4-pro" {
		return t.AccentPro
	}
	return t.AccentFlash
}

// maxReasoningLines caps the expanded reasoning panel so long chains of thought
// can't flood the viewport. Overflow collapses to a fgFaint "… N more lines"
// hint pointing at the ctrl+t toggle.
const maxReasoningLines = 16

// maxBodyLines is the collapsed height of a tool result / diff before it shows
// the "… N more lines, press e to expand" hint. It is a PACKAGE const so the
// renderer (which truncates here) and Scrollback.ExpandLastResult (which
// decides whether `e` has anything to reveal) share ONE threshold — otherwise
// a result longer than this but under the old hard-coded gate showed the hint
// yet refused to expand.
const maxBodyLines = 10

func (i chatItem) render(t Theme, width int) string {
	switch i.kind {
	case itemUser:
		// User message: brandDeep left bar + the DeepSeek '>' identity glyph.
		return barLine(t, t.BrandDeep, t.UserPrompt.Render("> "+i.text)) + "\n"
	case itemAssistantText:
		// Assistant prose is airy markdown on the canvas: NO left bar and NO
		// panel, just the shared 2-cell gutter so it lines up with barred
		// items. Render through Glamour so **bold** / `code` / lists /
		// headings get proper ANSI styling AND line wrapping respects the
		// terminal width (account for the gutter so long lines don't overflow).
		body := renderMarkdown(i.text, t, t.fillsEnabled(), width-len(t.Gutter()))
		return renderAssistantBody(t, body) + "\n"
	case itemReasoning:
		barColor := t.FgSubtle // dim bar for the latent thinking channel
		if i.folded {
			label := fmt.Sprintf("%s thinking %.1fs ~%d tok", IconFoldClosed, i.duration.Seconds(), i.tokens)
			line := t.ReasoningFold.Render(label)
			if peek := reasoningPeek(i.reasoning, 60); peek != "" {
				line += "  " + t.Reasoning.Render(peek)
			}
			return barLine(t, barColor, line) + t.Hint.Render("  [^R to expand]") + "\n"
		}
		// Expanded — reasoning is plain text, not markdown; wrap it and lay it
		// inside a surface panel, capped at maxReasoningLines visible rows.
		header := barLine(t, barColor, t.ReasoningFold.Render(
			fmt.Sprintf("%s thinking %.1fs ~%d tok", IconFoldOpen, i.duration.Seconds(), i.tokens)))
		panel := t.Panel(TierSurface).Foreground(t.FgSubtle).Italic(true)
		body := wrapWords(i.reasoning, width-len(t.Gutter()))
		lines := strings.Split(body, "\n")
		var b strings.Builder
		b.WriteString(header + "\n")
		shown := lines
		if len(lines) > maxReasoningLines {
			shown = lines[:maxReasoningLines]
		}
		for _, line := range shown {
			b.WriteString(gutterLine(t, panel.Render(line)) + "\n")
		}
		if len(lines) > maxReasoningLines {
			remaining := len(lines) - maxReasoningLines
			b.WriteString(gutterLine(t, t.Hint.Render(
				fmt.Sprintf("… %d more lines (ctrl+t)", remaining))) + "\n")
		}
		b.WriteString(gutterLine(t, t.Hint.Render(
			fmt.Sprintf("(%.1fs · ~%d tok · [^R to collapse])", i.duration.Seconds(), i.tokens))) + "\n")
		return b.String()
	case itemToolCall:
		// Tool CALL: route through RenderTool card dispatch for rich rendering.
		// RenderTool returns a complete card (head + optional body); wrap with
		// gutterLine for the universal 2-cell indent.
		innerW := width - len(t.Gutter())
		if innerW < 10 {
			innerW = 10
		}
		opts := ToolRenderOpts{
			Tool:     i.tool,
			Args:     i.args,
			Status:   ToolRunning,
			Expanded: i.expanded,
			Width:    innerW,
		}
		card := RenderTool(t, innerW, opts)
		return gutterLine(t, card) + "\n"
	case itemToolResult:
		// Tool RESULT: route through RenderTool card dispatch for rich
		// rendering. RenderTool returns a complete card (head + optional
		// body); wrap with gutterLine for the universal 2-cell indent.
		status := ToolSuccess
		if i.result.IsError {
			status = ToolError
		}
		innerW := width - len(t.Gutter())
		if innerW < 10 {
			innerW = 10
		}
		opts := ToolRenderOpts{
			Tool:     i.tool,
			Args:     i.args,
			Result:   i.result.Content,
			Status:   status,
			Expanded: i.expanded,
			Width:    innerW,
		}
		card := RenderTool(t, innerW, opts)
		return gutterLine(t, card) + "\n"
	case itemHookFired:
		style := t.HookInfo
		barColor := t.WarnColor
		if i.hookDecision == "deny" {
			style = t.HookDeny
			barColor = t.ErrColor
		}
		line := fmt.Sprintf("[hook %s] %s (%s): %s",
			i.tool, i.hookDecision,
			i.duration.Round(time.Millisecond).String(),
			i.hookReason)
		return barLine(t, barColor, style.Render(line)) + "\n"
	case itemRepair:
		// Compact one-line repair receipt with wrapping for long messages.
		// Format: "repaired tool call: <tool> <message>"
		// Example: "repaired tool call: read_file args completed"
		var toolPart string
		if i.repairTool != "" {
			toolPart = i.repairTool + " "
		}
		line := "repaired tool call: " + toolPart + i.repairMessage
		wrapped := wrapWords(line, width)
		return indent(t.Repair.Render(wrapped), t.Gutter()) + "\n"
	case itemInfo:
		// Notice line: NOTE chip + the message, under the shared gutter.
		return gutterLine(t, t.Badge(BadgeInfo).Render("NOTE")+" "+t.Info.Render(i.text)) + "\n"
	case itemStepFinish:
		// Cache% and ¥ are DeepSeek-only economics (the sole provider with a
		// prefix cache + pricing table). For a non-priced model omit both so the
		// line doesn't read a meaningless "cache=0% ¥?".
		var label string
		if llm.CostKnown(i.model) {
			cost := llm.Cost(i.model, i.usage)
			hit := llm.CacheHitRate(i.usage)
			label = fmt.Sprintf("[step done: %s · in=%d out=%d cache=%.0f%% ¥%.4f]",
				i.stopReason, i.usage.PromptTokens, i.usage.CompletionTokens, hit*100, cost)
		} else {
			label = fmt.Sprintf("[step done: %s · in=%d out=%d]",
				i.stopReason, i.usage.PromptTokens, i.usage.CompletionTokens)
		}
		return gutterLine(t, t.Status.Render(label)) + "\n"
	case itemError:
		// Error line: ERROR chip + the message, under the shared gutter.
		return gutterLine(t, t.Badge(BadgeErr).Render("ERROR")+" "+t.Error.Render(i.text)) + "\n"
	case itemWelcome:
		return renderWelcome(t, width)
	}
	return ""
}

// isNoticeTool reports whether a tool's result should carry the informational
// NOTE chip rather than the default success rendering — used for multi-edit /
// notice-style tools whose output is advisory, not a code/output payload.
func isNoticeTool(tool string) bool {
	switch tool {
	case "multi_edit", "multiedit", "notice", "note":
		return true
	default:
		return false
	}
}

// renderAssistantBody wraps an already-rendered markdown body in the
// assistant item's gutter/style chrome. Shared by the finalized path
// (items.go) and the streaming path (scrollback.go) so both produce
// identical chrome.
func renderAssistantBody(t Theme, mdBody string) string {
	return indent(mdBody, t.Gutter())
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

func oneline(s string, max int) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\n", "\\n"), "\t", "  ")
	if len(s) > max {
		s = s[:max-1] + "…"
	}
	return s
}

// compactArgs turns {"path":"foo","line":3} into 'path="foo", line=3'
// for readable inline display. Falls back to oneline if args isn't a
// well-formed JSON object.
func compactArgs(args string, max int) string {
	if max < 12 {
		max = 12
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(args), &m); err != nil || len(m) == 0 {
		return oneline(args, max)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		switch v := m[k].(type) {
		case string:
			parts = append(parts, fmt.Sprintf("%s=%q", k, v))
		default:
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
	}
	s := strings.Join(parts, ", ")
	runes := []rune(s)
	if len(runes) > max {
		s = string(runes[:max-1]) + "…"
	}
	return s
}

// highlightLang returns a chroma language identifier for the given tool
// name and arguments, or "" if no syntax highlighting applies.
func highlightLang(tool, args string) string {
	switch tool {
	case "bash":
		return "bash"
	case "edit_file", "apply_patch":
		return "diff"
	case "write_file":
		var m map[string]any
		if json.Unmarshal([]byte(args), &m) == nil {
			if p, ok := m["path"].(string); ok {
				if idx := strings.LastIndex(p, "."); idx >= 0 {
					return p[idx+1:]
				}
			}
		}
		return ""
	default:
		return ""
	}
}

// looksLikeDiff reports whether content is a unified/git diff, so it can be
// routed through the diff lexer regardless of which tool produced it (a
// `git diff` run via bash defaults to the shell lexer otherwise). It is
// deliberately conservative: it requires a structural marker — a
// "diff --git " line or a unified hunk header ("@@ … @@") — rather than mere
// leading +/- lines, so ordinary shell output or source code containing
// +/- operators is not misclassified.
func looksLikeDiff(content string) bool {
	if content == "" {
		return false
	}
	for _, ln := range strings.Split(content, "\n") {
		if strings.HasPrefix(ln, "diff --git ") {
			return true
		}
		// Unified hunk header: "@@ -a,b +c,d @@" (the trailing " @@" guards
		// against incidental lines that merely begin with "@@ ").
		if strings.HasPrefix(ln, "@@ ") && strings.Contains(ln[3:], " @@") {
			return true
		}
	}
	return false
}

// diffCodeLang infers the chroma language to syntax-highlight the code WITHIN
// a unified diff, from the diff's own file headers. It prefers the "+++ b/path"
// (new side) header, then "--- a/path", then the "diff --git a/x b/x" line,
// and maps the file extension to a chroma lexer name. Returns "" when no path
// is discoverable or the extension is unknown — diffview then draws the bands
// without syntax color.
func diffCodeLang(diff string) string {
	var newPath, oldPath, gitPath string
	for _, ln := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(ln, "+++ "):
			newPath = trimDiffPath(strings.TrimPrefix(ln, "+++ "))
		case strings.HasPrefix(ln, "--- "):
			oldPath = trimDiffPath(strings.TrimPrefix(ln, "--- "))
		case strings.HasPrefix(ln, "diff --git "):
			// "diff --git a/foo b/foo" → take the b/ side.
			fields := strings.Fields(strings.TrimPrefix(ln, "diff --git "))
			if len(fields) == 2 {
				gitPath = trimDiffPath(fields[1])
			}
		}
	}
	// Prefer the new side; fall back to the old side, then the git header.
	// Skip /dev/null sentinels (add/delete diffs) so we don't infer a lang
	// from the missing side.
	for _, p := range []string{newPath, oldPath, gitPath} {
		if p == "" || p == "/dev/null" {
			continue
		}
		idx := strings.LastIndex(p, ".")
		if idx < 0 || idx == len(p)-1 {
			continue
		}
		return p[idx+1:]
	}
	return ""
}

// trimDiffPath strips the conventional "a/" or "b/" prefix and any trailing
// tab-delimited metadata (timestamps) from a diff file-header path.
func trimDiffPath(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.IndexByte(s, '\t'); idx >= 0 {
		s = s[:idx]
	}
	if strings.HasPrefix(s, "a/") || strings.HasPrefix(s, "b/") {
		s = s[2:]
	}
	return s
}

// lineCount counts effective lines (ignores trailing blank line).
func lineCount(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

// reasoningPeek returns a single-line preview of the reasoning body
// for the collapsed-thinking summary chip. Returns "" for empty input.
// Strips newlines, trims whitespace, and ellipsis-truncates at max
// runes so the chip never wraps.
func reasoningPeek(s string, max int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	// Collapse runs of spaces for compactness.
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	runes := []rune(s)
	if len(runes) > max {
		s = string(runes[:max-1]) + "…"
	}
	return "\"" + s + "\""
}

// truncate keeps head + tail of long output with a marker, mirroring
// the bash tool's behavior so models and humans see the same shape.
func truncate(s string, maxLines, _ int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= maxLines {
		return s
	}
	keep := maxLines / 2
	head := lines[:keep]
	tail := lines[len(lines)-keep:]
	marker := fmt.Sprintf("... (truncated %d lines) ...", len(lines)-keep*2)
	return strings.Join(head, "\n") + "\n" + marker + "\n" + strings.Join(tail, "\n")
}
