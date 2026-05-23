package tui

import (
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/tools"
)

// TestRenderTrailingNewline guards against the bug where itemAssistantText
// rendered without a trailing newline, causing the next chat item to glue
// onto its last visual row and overflow past the viewport's right edge.
func TestRenderTrailingNewline(t *testing.T) {
	theme := DarkTheme()
	cases := []struct {
		name string
		item chatItem
	}{
		{"user", chatItem{kind: itemUser, text: "hi"}},
		{"assistant", chatItem{kind: itemAssistantText, text: "hello"}},
		{"reasoning folded", chatItem{kind: itemReasoning, folded: true}},
		{"reasoning expanded", chatItem{kind: itemReasoning, reasoning: "thought"}},
		{"tool call", chatItem{kind: itemToolCall, tool: "ls", args: "{}"}},
		{"tool result ok", chatItem{kind: itemToolResult, tool: "ls", result: tools.Result{Content: "out"}}},
		{"tool result empty", chatItem{kind: itemToolResult, tool: "ls", result: tools.Result{}}},
		{"duet approve", chatItem{kind: itemDuet, approved: true, duetReasoning: "ok"}},
		{"duet block", chatItem{kind: itemDuet, approved: false, duetReasoning: "no"}},
		{"info", chatItem{kind: itemInfo, text: "hi"}},
		{"step finish", chatItem{kind: itemStepFinish, model: "deepseek-v4-flash"}},
		{"error", chatItem{kind: itemError, text: "boom"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := c.item.render(theme, 80)
			if out == "" {
				t.Fatalf("render returned empty string")
			}
			if !strings.HasSuffix(out, "\n") {
				t.Errorf("render must end with \\n; got tail %q", out[max(0, len(out)-20):])
			}
		})
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
