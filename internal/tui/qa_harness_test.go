package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// QAFrame captures a named snapshot of a scrollback view for
// deterministic regression testing.
type QAFrame struct {
	Name string
	View string
}

// CaptureScrollbackFrame renders the scrollback at the given width
// and returns a QAFrame with normalized trailing whitespace.
func CaptureScrollbackFrame(name string, s *Scrollback, width int) QAFrame {
	view := s.Render(DarkTheme(), width)
	// Normalize trailing whitespace on each line
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return QAFrame{
		Name: name,
		View: strings.Join(lines, "\n"),
	}
}

// assertFrameSnapshot compares the frame view against an inline expected
// string after stripping ANSI codes and normalizing whitespace.
func assertFrameSnapshot(t *testing.T, frame QAFrame, expected string) {
	t.Helper()
	actual := stripANSI(frame.View)
	expected = strings.TrimSpace(expected)
	actual = strings.TrimSpace(actual)
	if actual != expected {
		t.Errorf("frame %q snapshot mismatch:\n--- expected ---\n%s\n--- actual ---\n%s", frame.Name, expected, actual)
	}
}

func TestQAFrame_EmptyScrollback(t *testing.T) {
	sb := NewScrollback()
	frame := CaptureScrollbackFrame("empty", sb, 80)
	if frame.Name != "empty" {
		t.Errorf("frame.Name = %q, want %q", frame.Name, "empty")
	}
	// Empty scrollback should produce empty output
	if frame.View != "" {
		t.Errorf("expected empty frame, got %q", frame.View)
	}
}

func TestQAFrame_ReasoningFolded(t *testing.T) {
	sb := NewScrollback()
	sb.AppendReasoning("This is my thinking process...")

	frame := CaptureScrollbackFrame("reasoning_folded", sb, 80)

	// Inline expected string for folded reasoning: dim left bar + fold glyph +
	// "thinking Xs ~N tok" + a short quoted preview + the expand hint. (The
	// leading gutter + bar opens the line; assertFrameSnapshot trims the
	// outer whitespace so the bar leads here.)
	expected := `▌ ▸ thinking 0.0s ~7 tok  "This is my thinking process..."  [^R to expand]`
	assertFrameSnapshot(t, frame, expected)
}

func TestQAFrame_ToolCall(t *testing.T) {
	sb := NewScrollback()
	sb.AppendToolCall("c1", "read_file", `{"path":"test.go"}`)

	frame := CaptureScrollbackFrame("tool_call", sb, 80)

	// Inline expected string for tool call: gutter + model-accent bar +
	// status glyph + summary. assertFrameSnapshot trims the outer gutter.
	expected := "▌ ● read test.go"
	assertFrameSnapshot(t, frame, expected)
}

func TestQAFrame_ToolResult(t *testing.T) {
	sb := NewScrollback()
	sb.AppendToolCall("c1", "read_file", `{"path":"test.go"}`)
	sb.AppendToolResult("c1", tools.Result{Content: "package main\n\nfunc main() {}"}, 100*time.Millisecond)

	frame := CaptureScrollbackFrame("tool_result", sb, 80)
	view := stripANSI(frame.View)

	// Print actual output for debugging
	t.Logf("Actual tool result frame:\n%s", view)

	// Inline expected string for tool result: the barred CALL header, then the
	// barred RESULT header (model accent + status glyph + summary + duration),
	// then the panelled body lines under the shared 2-cell gutter (no per-line
	// bar). assertFrameSnapshot strips ANSI (so the panel background is not
	// visible) and trims outer whitespace (so the first line's gutter drops).
	// Duration is right-aligned to width 80. The header line:
	// gutter(2) + bar(1) + space(1) + icon(1) + space(1) + summary(22) + padding + dur(5) = 80
	// prefix(6) + summary(22) + padding + dur(5) = 80 → padding = 47.
	expected := "▌ ● read test.go\n  ▌ ✓ read test.go (3 lines)" + strings.Repeat(" ", 47) + "100ms\n  package main\n  \n  func main() {}"
	assertFrameSnapshot(t, frame, expected)
}

func TestQAFrame_ToolResultError(t *testing.T) {
	sb := NewScrollback()
	sb.AppendToolCall("c1", "bash", `{"command":"rm -rf /"}`)
	sb.AppendToolResult("c1", tools.Result{Content: "permission denied", IsError: true}, 50*time.Millisecond)

	frame := CaptureScrollbackFrame("tool_result_error", sb, 80)
	view := stripANSI(frame.View)

	// Error tool result should have error icon
	if !strings.Contains(view, "✗") {
		t.Errorf("expected error icon '✗' in frame, got %q", view)
	}
	if !strings.Contains(view, "permission denied") {
		t.Errorf("expected 'permission denied' in frame, got %q", view)
	}
	if !strings.Contains(view, "bash: rm -rf /") {
		t.Errorf("expected 'bash: rm -rf /' in frame, got %q", view)
	}
}

func TestQAFrame_RepairReceipt(t *testing.T) {
	sb := NewScrollback()
	sb.AppendRepair("args_completed", "read_file", "args completed")

	frame := CaptureScrollbackFrame("repair", sb, 80)
	view := stripANSI(frame.View)

	// Repair receipt should have exactly one compact line
	lines := strings.Split(strings.TrimSpace(view), "\n")
	nonEmptyLines := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			nonEmptyLines++
		}
	}
	if nonEmptyLines != 1 {
		t.Errorf("expected exactly 1 compact repair line, got %d lines: %q", nonEmptyLines, view)
	}

	// Inline expected string for repair receipt
	expected := "repaired tool call: read_file args completed"
	assertFrameSnapshot(t, frame, expected)
}

func TestQAFrame_WithStepFinish(t *testing.T) {
	sb := NewScrollback()
	sb.AppendUser("hello")
	sb.AppendText("I'm thinking...")
	sb.AppendStepFinish("stop", llm.Usage{
		PromptCacheHitTokens:  100,
		PromptCacheMissTokens: 50,
		CompletionTokens:      200,
	}, "flash")

	frame := CaptureScrollbackFrame("step_finish", sb, 80)
	view := stripANSI(frame.View)

	// Print actual output for debugging
	t.Logf("Actual step finish frame:\n%s", view)

	// Verify key content is present
	if !strings.Contains(view, "hello") {
		t.Errorf("expected 'hello' in frame, got %q", view)
	}
	if !strings.Contains(view, "thinking") {
		t.Errorf("expected 'thinking' in frame, got %q", view)
	}
	if !strings.Contains(view, "step done") {
		t.Errorf("expected 'step done' in frame, got %q", view)
	}
	if !strings.Contains(view, "out=200") {
		t.Errorf("expected 'out=200' in frame, got %q", view)
	}
}

func TestQAFrame_HUDSnapshot(t *testing.T) {
	// Test RenderHUD directly with a QAFrame snapshot
	hudData := HUDData{
		Model:           "flash",
		InputHitTokens:  100,
		InputMissTokens: 50,
		OutputTokens:    200,
		StepCNY:         0.013,
	}

	frame := QAFrame{Name: "hud", View: RenderHUD(hudData, 80)}
	expected := "flash | cache 66.7% | out 200 | ¥0.013"
	assertFrameSnapshot(t, frame, expected)
}

func TestQAFrame_WidthDeterministic(t *testing.T) {
	sb := NewScrollback()
	sb.AppendUser("hello world this is a long message that might wrap")

	// Capture at different widths
	frame80 := CaptureScrollbackFrame("wide", sb, 80)
	frame40 := CaptureScrollbackFrame("narrow", sb, 40)

	// Both should succeed without panic
	if frame80.View == "" {
		t.Error("expected non-empty frame at width 80")
	}
	if frame40.View == "" {
		t.Error("expected non-empty frame at width 40")
	}

	// Capture again at same width - should be deterministic
	frame80b := CaptureScrollbackFrame("wide", sb, 80)
	if frame80.View != frame80b.View {
		t.Errorf("frame not deterministic at width 80:\n%s\nvs\n%s", frame80.View, frame80b.View)
	}
}

func TestQAFrame_TrailingWhitespaceNormalized(t *testing.T) {
	sb := NewScrollback()
	sb.AppendUser("hello")

	frame := CaptureScrollbackFrame("whitespace", sb, 80)

	// Check that no line has trailing whitespace
	lines := strings.Split(frame.View, "\n")
	for _, line := range lines {
		if line != strings.TrimRight(line, " \t") {
			t.Errorf("line has trailing whitespace: %q", line)
		}
	}
}
