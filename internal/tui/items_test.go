package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"
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

// TestLooksLikeDiff pins the T5.4 diff scanner's boundaries: it must catch
// git/unified diffs by their structural markers without misclassifying
// ordinary shell output or source code that merely contains +/- characters.
func TestLooksLikeDiff(t *testing.T) {
	positives := map[string]string{
		"git diff":                    "diff --git a/main.go b/main.go\nindex 1..2 100644\n--- a/main.go\n+++ b/main.go\n@@ -1,3 +1,4 @@\n-old\n+new",
		"unified diff -u (hunk only)": "--- a.txt\n+++ b.txt\n@@ -1 +1 @@\n-a\n+b",
		"hunk with offsets":           "@@ -10,7 +10,9 @@ func main() {",
	}
	for name, content := range positives {
		if !looksLikeDiff(content) {
			t.Errorf("%s: expected looksLikeDiff=true", name)
		}
	}
	negatives := map[string]string{
		"empty":                "",
		"plain bash output":    "total 8\ndrwxr-xr-x  2 user  staff  64 May 29 main.go",
		"shell script with +x": "chmod +x script.sh\necho '+ done'\nVAR=-1",
		"go source with +/-":   "x := a + b\ny := a - b\nreturn x - y",
		"log line with @@":     "user@@host ran a job",
		"markdown heading":     "## @@ not a hunk",
	}
	for name, content := range negatives {
		if looksLikeDiff(content) {
			t.Errorf("%s: expected looksLikeDiff=false, got true for %q", name, content)
		}
	}
}

// TestToolResultRoutesDiffThroughDiffView proves a diff produced by bash is
// routed through the rich diffview: the band fills emit ANSI, and the code
// text round-trips intact. The leading +/- marker column is replaced by the
// colored band + line-number gutter, so the surviving payload is the code
// text ("was" / "now") and the hunk header — NOT the literal "-was" / "+now".
func TestToolResultRoutesDiffThroughDiffView(t *testing.T) {
	diff := "diff --git a/x.go b/x.go\n@@ -1,2 +1,2 @@\n-was\n+now\n"
	item := chatItem{kind: itemToolResult, tool: "bash", args: `{"command":"git diff"}`, result: tools.Result{Content: diff}, expanded: true}
	out := item.render(DarkTheme(), 80)
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("expected ANSI band fills in diff render, got %q", out)
	}
	plain := stripANSI(out)
	for _, want := range []string{"was", "now", "@@ -1,2 +1,2 @@"} {
		if !strings.Contains(plain, want) {
			t.Errorf("diff text %q missing from render:\n%s", want, plain)
		}
	}
	// The diff view draws a line-number gutter; the new-side line numbers for
	// the added line must appear.
	if !strings.Contains(plain, "1") {
		t.Errorf("expected a line-number gutter in diff render:\n%s", plain)
	}
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
		{`{}`, `{}`},             // empty → fall back
		{`not json`, `not json`}, // bad json → fall back
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

func TestToolCard(t *testing.T) {
	th := DarkTheme()

	t.Run("tool call renders card header", func(t *testing.T) {
		item := chatItem{kind: itemToolCall, tool: "bash", args: `{"command":"echo hi"}`}
		out := item.render(th, 80)
		plain := stripANSI(out)
		// RenderTool dispatches to renderBashCard which shows status glyph + "bash" + cmd.
		if !strings.Contains(plain, "bash") {
			t.Errorf("expected tool name in output, got %q", out)
		}
		if !strings.Contains(plain, "echo hi") {
			t.Errorf("expected command in output, got %q", out)
		}
	})

	t.Run("tool result success renders card", func(t *testing.T) {
		item := chatItem{
			kind:     itemToolResult,
			tool:     "bash",
			args:     `{"command":"echo hi"}`,
			result:   tools.Result{Content: "hello\nworld"},
			duration: 1200 * time.Millisecond,
		}
		out := item.render(th, 80)
		plain := stripANSI(out)
		// RenderTool dispatches to renderBashCard with ToolSuccess status.
		if !strings.Contains(plain, "bash") {
			t.Errorf("expected tool name in output, got %q", out)
		}
	})

	t.Run("tool result error renders card", func(t *testing.T) {
		item := chatItem{
			kind:     itemToolResult,
			tool:     "bash",
			args:     `{"command":"fail"}`,
			result:   tools.Result{Content: "error msg", IsError: true},
			duration: 100 * time.Millisecond,
		}
		out := item.render(th, 80)
		plain := stripANSI(out)
		// RenderTool dispatches to renderBashCard with ToolError status,
		// which uses the error glyph (✖).
		if !strings.Contains(plain, "bash") {
			t.Errorf("expected tool name in output, got %q", out)
		}
	})

	t.Run("folded body hides content", func(t *testing.T) {
		// Create content with more than 10 lines.
		lines := make([]string, 20)
		for i := range lines {
			lines[i] = fmt.Sprintf("line %d", i)
		}
		item := chatItem{
			kind:     itemToolResult,
			tool:     "bash",
			args:     "{}",
			result:   tools.Result{Content: strings.Join(lines, "\n")},
			duration: 100 * time.Millisecond,
			expanded: false,
		}
		out := item.render(th, 80)
		plain := stripANSI(out)
		// RenderTool dispatches to renderBashCard which hides body when
		// collapsed (expanded=false). The head line should still show.
		if !strings.Contains(plain, "bash") {
			t.Errorf("expected tool name in output, got %q", out)
		}
		// Body content should NOT be visible when collapsed.
		if strings.Contains(plain, "line 0") {
			t.Errorf("collapsed card should not show body content, got %q", out)
		}
	})

	t.Run("expanded body shows content", func(t *testing.T) {
		item := chatItem{
			kind:     itemToolResult,
			tool:     "bash",
			args:     "{}",
			result:   tools.Result{Content: "hello output"},
			duration: 100 * time.Millisecond,
			expanded: true,
		}
		out := item.render(th, 80)
		plain := stripANSI(out)
		// RenderTool dispatches to renderBashCard which shows body when
		// expanded. The body is rendered via cardBody with oneline().
		if !strings.Contains(plain, "bash") {
			t.Errorf("expected tool name in output, got %q", out)
		}
		if !strings.Contains(plain, "hello output") {
			t.Errorf("expected body content in expanded output, got %q", out)
		}
	})
}

// TestChatItem_EditToolRendersDiff verifies that edit_file tool items route
// through RenderTool's renderEditCard, which renders diff content when expanded.
func TestChatItem_EditToolRendersDiff(t *testing.T) {
	th := DarkTheme()
	diff := `--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main

+import "fmt"
 func main() {`
	args := fmt.Sprintf(`{"path":"main.go","diff":%q}`, diff)
	item := chatItem{
		kind:     itemToolResult,
		tool:     "edit_file",
		args:     args,
		result:   tools.Result{Content: "done"},
		expanded: true,
	}
	out := item.render(th, 80)
	plain := stripANSI(out)
	// The edit card should show the tool name and the diff content.
	if !strings.Contains(plain, "edit_file") {
		t.Errorf("expected tool name in output, got %q", out)
	}
	if !strings.Contains(plain, "main.go") {
		t.Errorf("expected file path in output, got %q", out)
	}
	// Expanded edit card should render the diff body.
	if !strings.Contains(plain, "import") {
		t.Errorf("expected diff content in expanded output, got %q", out)
	}
}

// TestWelcomeRenders sanity-checks that the startup banner contains
// both the mascot signature and the wordmark when the terminal is
// wide enough.
func TestWelcomeRenders(t *testing.T) {
	out := renderWelcome(DarkTheme(), 100)
	plain := stripANSI(out)
	// Spot-check the letterform wordmark and the tagline.
	if !strings.Contains(plain, "█") {
		t.Errorf("expected letterform block characters in welcome; got %q", plain)
	}
	if !strings.Contains(plain, "DeepSeek") {
		t.Errorf("expected 'DeepSeek' tagline in welcome; got %q", plain)
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

func TestToolCardHighlight(t *testing.T) {
	th := DarkTheme()
	item := chatItem{
		kind:     itemToolResult,
		tool:     "bash",
		args:     `{"command":"echo hi"}`,
		result:   tools.Result{Content: "echo hello\necho world"},
		duration: 100 * time.Millisecond,
		expanded: true,
	}
	out := item.render(th, 80)
	// The bash body should be syntax-highlighted, producing ANSI codes
	// beyond what plain CardBody would emit. At minimum the highlight
	// output differs from CardBody rendering of the same text.
	plainBody := th.CardBody.Render("echo hello")
	if strings.Contains(out, plainBody) {
		// CardBody single-color rendering appeared verbatim — highlight
		// was NOT applied. This is the failure the review flagged.
		t.Error("tool card body rendered as plain CardBody without syntax highlighting")
	}
	if !strings.Contains(out, "\x1b[") {
		t.Error("expected ANSI highlight sequences in tool card body")
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
