package tui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

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
		return fmt.Sprintf("tool-result:%s:%s:%t:%d:%s", i.toolCallID, theme, i.expanded, width, i.result.Content)
	case itemToolCall:
		return fmt.Sprintf("tool-call:%s:%s:%d:%s", i.toolCallID, theme, width, i.args)
	default:
		return ""
	}
}

func (i chatItem) render(t Theme, width int) string {
	switch i.kind {
	case itemUser:
		return t.UserPrompt.Render("> "+i.text) + "\n"
	case itemAssistantText:
		// Assistant output is markdown — render through Glamour so
		// **bold** / `code` / lists / headings get proper ANSI styling
		// AND the line wrapping respects the terminal width. Without
		// this, long lines overflow past the viewport's right edge.
		body := renderMarkdown(i.text, t.Name, width)
		return body + "\n"
	case itemReasoning:
		if i.folded {
			label := fmt.Sprintf("▸ thinking (%.1fs · ~%d tok)", i.duration.Seconds(), i.tokens)
			if peek := reasoningPeek(i.reasoning, 60); peek != "" {
				label += "  " + t.Reasoning.Render(peek)
			}
			return t.ReasoningFold.Render(label) + t.Hint.Render("  [^R to expand]") + "\n"
		}
		// expanded — reasoning is plain text, not markdown; just wrap.
		body := wrapWords(i.reasoning, width-2)
		return t.ReasoningFold.Render("▾ thinking") + "\n" +
			t.Reasoning.Render(indent(body, "  ")) + "\n" +
			t.Hint.Render(fmt.Sprintf("  (%.1fs · ~%d tok · [^R to collapse])", i.duration.Seconds(), i.tokens)) + "\n"
	case itemToolCall:
		// Card header: ▌ ● toolName  args (pending state)
		prefix := t.CardBar.Render(BarThick + " ")
		mcpTag := ""
		if strings.HasPrefix(i.tool, "mcp__") {
			mcpTag = "[MCP] "
		}
		// Use RenderToolSummary for consistent tool rendering
		summary := RenderToolSummary(i.tool, i.args, "", false, width-8)
		return prefix + t.ToolCall.Render(IconToolPending+" "+mcpTag+summary) + "\n"
	case itemToolResult:
		// Card: header line with status + duration, then body lines.
		prefix := t.CardBar.Render(BarThick + " ")
		indent := t.CardBar.Render(BarThick) + "  "

		dur := i.duration.Round(time.Millisecond).String()
		mcpTag := ""
		if strings.HasPrefix(i.tool, "mcp__") {
			mcpTag = "[MCP] "
		}

		// Use RenderToolSummary for consistent tool rendering
		summary := RenderToolSummary(i.tool, i.args, i.result.Content, i.result.IsError, width-len(dur)-10)

		// Header line with status icon + summary + duration.
		var statusIcon string
		var statusStyle lipgloss.Style
		if i.result.IsError {
			statusIcon = IconToolErr
			statusStyle = t.ToolErr
		} else {
			statusIcon = IconToolOk
			statusStyle = t.ToolOk
		}
		header := prefix +
			statusStyle.Render(statusIcon+" "+mcpTag+summary) +
			"  " + statusStyle.Render(dur) + "\n"

		body := i.result.Content
		if body == "" {
			return header
		}

		// Body lines with card bar prefix.
		const maxBodyLines = 10
		lang := highlightLang(i.tool, i.args)
		bodyLines := strings.Split(body, "\n")
		if lang != "" {
			highlighted := Highlight(t, body, lang)
			bodyLines = strings.Split(highlighted, "\n")
		}
		if !i.expanded && len(bodyLines) > maxBodyLines {
			// Folded: show first maxBodyLines + truncation hint.
			var b strings.Builder
			b.WriteString(header)
			for _, line := range bodyLines[:maxBodyLines] {
				b.WriteString(indent + t.CardBody.Render(line) + "\n")
			}
			remaining := len(bodyLines) - maxBodyLines
			b.WriteString(indent + t.Hint.Render(fmt.Sprintf("… %d more lines, press e to expand", remaining)) + "\n")
			return b.String()
		}
		// Expanded or short body.
		var b strings.Builder
		b.WriteString(header)
		for _, line := range bodyLines {
			b.WriteString(indent + t.CardBody.Render(line) + "\n")
		}
		return b.String()
	case itemHookFired:
		style := t.HookInfo
		if i.hookDecision == "deny" {
			style = t.HookDeny
		}
		line := fmt.Sprintf("[hook %s] %s (%s): %s",
			i.tool, i.hookDecision,
			i.duration.Round(time.Millisecond).String(),
			i.hookReason)
		return style.Render(line) + "\n"
	case itemRepair:
		// Compact one-line repair receipt with wrapping for long messages.
		// Format: "repaired tool call: <tool> <message>"
		// Example: "repaired tool call: read_file args completed"
		var toolPart string
		if i.repairTool != "" {
			toolPart = i.repairTool + " "
		}
		line := "repaired tool call: " + toolPart + i.repairMessage
		return t.Repair.Render(wrapWords(line, width)) + "\n"
	case itemInfo:
		return t.Info.Render("[info] "+i.text) + "\n"
	case itemStepFinish:
		cost := llm.Cost(i.model, i.usage)
		hit := llm.CacheHitRate(i.usage)
		costStr := "¥?"
		if llm.CostKnown(i.model) {
			costStr = fmt.Sprintf("¥%.4f", cost)
		}
		label := fmt.Sprintf("[step done: %s · in=%d out=%d cache=%.0f%% %s]",
			i.stopReason, i.usage.PromptTokens, i.usage.CompletionTokens, hit*100, costStr)
		return t.Status.Render(label) + "\n"
	case itemError:
		return t.Error.Render("error: "+i.text) + "\n"
	case itemWelcome:
		return renderWelcome(t, width)
	}
	return ""
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
