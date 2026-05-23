package tui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// itemKind enumerates the kinds of items the scrollback renders. New
// kinds added here should pair with a case in chatItem.render().
type itemKind int

const (
	itemUser itemKind = iota + 1
	itemAssistantText
	itemReasoning
	itemToolCall
	itemToolResult
	itemDuet
	itemInfo
	itemStepFinish
	itemError
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
	tokens    int           // rough estimate for collapsed label
	duration  time.Duration

	// ToolCall / ToolResult
	toolCallID string
	tool       string
	args       string
	result     tools.Result

	// Duet
	approved      bool
	duetReasoning string

	// StepFinish
	stopReason string
	usage      llm.Usage
	model      string
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
			return t.ReasoningFold.Render(label) + t.Hint.Render("  [^R to expand]") + "\n"
		}
		// expanded — reasoning is plain text, not markdown; just wrap.
		body := wrapWords(i.reasoning, width-2)
		return t.ReasoningFold.Render("▾ thinking") + "\n" +
			t.Reasoning.Render(indent(body, "  ")) + "\n" +
			t.Hint.Render(fmt.Sprintf("  (%.1fs · ~%d tok · [^R to collapse])", i.duration.Seconds(), i.tokens)) + "\n"
	case itemToolCall:
		// Claude-Code-style header: ⏺ tool(k1="v1", k2=v2) — tool name
		// in accent color, args humanized + dimmed in parens.
		argsMax := width - len(i.tool) - 6
		args := compactArgs(i.args, argsMax)
		return t.ToolCall.Render("⏺ "+i.tool) +
			t.Hint.Render("("+args+")") + "\n"
	case itemToolResult:
		// Connector + summary line: "  ⎿ N lines · 42ms" (or "✗ error · ...")
		dur := i.duration.Round(time.Millisecond).String()
		var summary string
		if i.result.IsError {
			summary = t.ToolErr.Render("✗ error · " + dur)
		} else {
			n := lineCount(i.result.Content)
			switch {
			case n == 0:
				summary = t.ToolOk.Render("✓ " + dur)
			case n == 1:
				summary = t.ToolOk.Render("✓ 1 line · " + dur)
			default:
				summary = t.ToolOk.Render(fmt.Sprintf("✓ %d lines · %s", n, dur))
			}
		}
		head := "  " + t.Hint.Render("⎿ ") + summary + "\n"

		body := i.result.Content
		if body == "" {
			return head
		}
		// Body indented 4 cols to align under the ⎿ connector.
		truncated := truncate(body, 30, width-4)
		return head + t.ToolBody.Render(indent(truncated, "    ")) + "\n"
	case itemDuet:
		head := "approved"
		style := t.DuetApprove
		if !i.approved {
			head = "BLOCKED"
			style = t.DuetBlock
		}
		line := fmt.Sprintf("◆ pro check (%s): %s — %s",
			i.duration.Round(time.Millisecond), head, i.duetReasoning)
		return style.Render(line) + "\n"
	case itemInfo:
		return t.Info.Render("[info] "+i.text) + "\n"
	case itemStepFinish:
		cost := llm.Cost(i.model, i.usage)
		hit := llm.CacheHitRate(i.usage)
		label := fmt.Sprintf("[step done: %s · in=%d out=%d cache=%.0f%% ¥%.4f]",
			i.stopReason, i.usage.PromptTokens, i.usage.CompletionTokens, hit*100, cost)
		return t.Status.Render(label) + "\n"
	case itemError:
		return t.Error.Render("error: "+i.text) + "\n"
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
