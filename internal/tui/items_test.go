package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

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
		{"welcome wide", chatItem{kind: itemWelcome}},
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

// TestAssistantTextRendersMarkdown guards against regressing back to
// raw `**bold**` literals showing in the scrollback. Glamour should
// consume the markdown syntax and emit ANSI-styled output.
func TestAssistantTextRendersMarkdown(t *testing.T) {
	item := chatItem{kind: itemAssistantText, text: "Hello **world**"}
	out := item.render(DarkTheme(), 80)

	if strings.Contains(out, "**") {
		t.Errorf("markdown asterisks leaked into render: %q", out)
	}
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("expected ANSI escape codes from Glamour, got plain text: %q", out)
	}
}

// TestHeadingPrefixesStripped pins the Claude-Code-style clean
// heading rendering: no literal "### " or "## " noise before the
// heading text. Bold + color still applies via the StylePrimitive.
func TestHeadingPrefixesStripped(t *testing.T) {
	src := "# H1\n## H2\n### H3\n#### H4\n"
	item := chatItem{kind: itemAssistantText, text: src}
	out := item.render(DarkTheme(), 80)

	for _, marker := range []string{"# ", "## ", "### ", "#### "} {
		if strings.Contains(out, marker) {
			t.Errorf("heading prefix %q leaked into render: %q", marker, out)
		}
	}
}

func TestCompactArgs(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`{"path":"cmd/dsc/main.go"}`, `path="cmd/dsc/main.go"`},
		{`{"path":"a","line":3}`, `line=3, path="a"`}, // sorted keys
		{`{}`, `{}`},                                   // empty → fall back
		{`not json`, `not json`},                       // bad json → fall back
	}
	for _, c := range cases {
		got := compactArgs(c.in, 200)
		if got != c.want {
			t.Errorf("compactArgs(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCompactArgsTruncates(t *testing.T) {
	long := `{"path":"` + strings.Repeat("x", 500) + `"}`
	got := compactArgs(long, 40)
	if n := utf8.RuneCountInString(got); n > 40 {
		t.Errorf("compactArgs did not respect max: runes=%d, got=%q", n, got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected truncation marker …, got %q", got)
	}
}

// TestRebindStopsAtTurnBoundary pins the fix for the bug where a new
// turn's text deltas were extending the previous turn's assistant-text
// block — making the user prompt appear AFTER its own response in the
// scrollback.
func TestRebindStopsAtTurnBoundary(t *testing.T) {
	a := &App{
		items: []chatItem{
			{kind: itemUser, text: "hello"},
			{kind: itemAssistantText, text: "world"},
			{kind: itemStepFinish, model: "x"},
			{kind: itemUser, text: "what next"},
		},
	}
	a.rebindStreamPointers()
	if a.streamText != nil {
		t.Errorf("streamText must be nil after a new user message; got pointer to %+v", *a.streamText)
	}
	if a.streamThink != nil {
		t.Errorf("streamThink must be nil after a new user message; got pointer to %+v", *a.streamThink)
	}
}

// TestRebindFindsCurrentTurnStream confirms the boundary fix did not
// regress the legitimate case: when the most recent items belong to
// the current in-progress turn, streamText must point to the trailing
// assistant-text block so subsequent text deltas extend it correctly.
func TestRebindFindsCurrentTurnStream(t *testing.T) {
	a := &App{
		items: []chatItem{
			{kind: itemUser, text: "hello"},
			{kind: itemReasoning, folded: true, reasoning: "thinking…"},
			{kind: itemAssistantText, text: "partial"},
		},
	}
	a.rebindStreamPointers()
	if a.streamText == nil {
		t.Fatalf("streamText should point at the trailing asst text item, got nil")
	}
	if a.streamText.text != "partial" {
		t.Errorf("streamText should point at the 'partial' item; got %q", a.streamText.text)
	}
	if a.streamThink == nil || a.streamThink.reasoning != "thinking…" {
		t.Errorf("streamThink should point at the reasoning item; got %+v", a.streamThink)
	}
}

// TestRebindStopsAtStepFinish ensures multi-step ReAct flows don't
// bleed text from a finished step into the next step's text block.
func TestRebindStopsAtStepFinish(t *testing.T) {
	a := &App{
		items: []chatItem{
			{kind: itemUser, text: "do X"},
			{kind: itemAssistantText, text: "step1 reply"},
			{kind: itemStepFinish, model: "x"},
		},
	}
	a.rebindStreamPointers()
	if a.streamText != nil {
		t.Errorf("streamText must be nil after stepFinish; got %+v", *a.streamText)
	}
}

// TestWelcomeRenders sanity-checks that the startup banner contains
// both the mascot signature and the wordmark when the terminal is
// wide enough.
func TestWelcomeRenders(t *testing.T) {
	out := renderWelcome(DarkTheme(), 100)
	// Spot-check the wordmark block-font (top row) and the tagline.
	if !strings.Contains(out, "____") {
		t.Errorf("expected wordmark block characters in welcome; got %q", out)
	}
	if !strings.Contains(out, "DeepSeek") {
		t.Errorf("expected 'DeepSeek' tagline in welcome; got %q", out)
	}
}

// TestWelcomeNarrowFallback ensures narrow terminals get a compact
// greeting instead of a wrapped ASCII art.
func TestWelcomeNarrowFallback(t *testing.T) {
	out := renderWelcome(DarkTheme(), 40)
	if strings.Count(out, "\n") > 2 {
		t.Errorf("narrow fallback should be at most two lines; got %q", out)
	}
	if !strings.Contains(out, "deepseekcode") {
		t.Errorf("narrow fallback missing name; got %q", out)
	}
}

func TestLineCount(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"one", 1},
		{"one\n", 1},
		{"one\ntwo", 2},
		{"one\ntwo\n", 2},
		{"a\nb\nc\nd", 4},
	}
	for _, c := range cases {
		if got := lineCount(c.in); got != c.want {
			t.Errorf("lineCount(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}
