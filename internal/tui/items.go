package tui

import (
	"fmt"
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
		// Trailing newline is load-bearing: without it the next chat
		// item glues onto the last visual row of the assistant text and
		// overflows past the viewport's right edge (showing as "[st" or
		// similar truncation artifacts).
		return t.AssistantText.Render(i.text) + "\n"
	case itemReasoning:
		if i.folded {
			label := fmt.Sprintf("▸ thinking (%.1fs · ~%d tok)", i.duration.Seconds(), i.tokens)
			return t.ReasoningFold.Render(label) + t.Hint.Render("  [^R to expand]") + "\n"
		}
		// expanded
		body := wrap(i.reasoning, width-2)
		return t.ReasoningFold.Render("▾ thinking") + "\n" +
			t.Reasoning.Render(indent(body, "  ")) + "\n" +
			t.Hint.Render(fmt.Sprintf("  (%.1fs · ~%d tok · [^R to collapse])", i.duration.Seconds(), i.tokens)) + "\n"
	case itemToolCall:
		return t.ToolCall.Render(fmt.Sprintf("▶ %s(%s)", i.tool, oneline(i.args, 100))) + "\n"
	case itemToolResult:
		var head string
		if i.result.IsError {
			head = t.ToolErr.Render(fmt.Sprintf("✗ %s (%s)", i.tool, i.duration.Round(time.Millisecond)))
		} else {
			head = t.ToolOk.Render(fmt.Sprintf("✓ %s (%s)", i.tool, i.duration.Round(time.Millisecond)))
		}
		body := i.result.Content
		if len(body) > 0 {
			return head + "\n" + t.ToolBody.Render(indent(truncate(body, 30, width-2), "  ")) + "\n"
		}
		return head + "\n"
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

func wrap(s string, width int) string {
	if width <= 10 || len(s) == 0 {
		return s
	}
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		if len(line) <= width {
			b.WriteString(line)
			b.WriteByte('\n')
			continue
		}
		// hard wrap on width; preserves indentation visually
		for len(line) > width {
			b.WriteString(line[:width])
			b.WriteByte('\n')
			line = line[width:]
		}
		if len(line) > 0 {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return strings.TrimRight(b.String(), "\n")
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
