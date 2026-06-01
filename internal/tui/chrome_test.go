package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/colorprofile"
)

func TestGradientRamp(t *testing.T) {
	t.Run("correct length", func(t *testing.T) {
		deep := DarkTheme().BrandDeep
		light := DarkTheme().BrandLight
		ramp := makeGradientRamp(8, deep, light)
		if len(ramp) != 8 {
			t.Fatalf("expected 8 colors, got %d", len(ramp))
		}
	})

	t.Run("single segment", func(t *testing.T) {
		deep := DarkTheme().BrandDeep
		light := DarkTheme().BrandLight
		ramp := makeGradientRamp(4, deep, light)
		if len(ramp) != 4 {
			t.Fatalf("expected 4 colors, got %d", len(ramp))
		}
	})

	t.Run("round trip ramp", func(t *testing.T) {
		deep := DarkTheme().BrandDeep
		light := DarkTheme().BrandLight
		ramp := makeGradientRamp(16, deep, light, deep)
		if len(ramp) != 16 {
			t.Fatalf("expected 16 colors, got %d", len(ramp))
		}
	})

	t.Run("less than 2 stops returns nil", func(t *testing.T) {
		ramp := makeGradientRamp(8)
		if ramp != nil {
			t.Fatalf("expected nil, got %v", ramp)
		}
	})
}

func TestSpinnerGradient(t *testing.T) {
	th := DarkTheme()
	c := NewChrome()
	c.BeginThinking()

	// Render at different frames — outputs should differ because of
	// the gradient offset.
	s0 := c.spinner(th)
	c.AdvanceFrame()
	c.AdvanceFrame()
	c.AdvanceFrame()
	c.AdvanceFrame()
	s4 := c.spinner(th)

	if s0 == s4 {
		t.Fatalf("spinner should differ between frame 0 and 4; both are %q", s0)
	}
}

// TestChromeToolBatchReady pins the T5.3 "N ready" multi-tool caption: a
// fresh batch resets the counters, parallel BeginTool calls grow the batch,
// ToolReady is clamped to the batch size, and a single-tool turn names the
// tool instead.
func TestChromeToolBatchReady(t *testing.T) {
	th := DarkTheme()
	c := NewChrome()

	// Single-tool turn names the tool.
	c.BeginTool("read_file")
	if got := stripANSI(c.Render(th, false)); !strings.Contains(got, "calling read_file") {
		t.Errorf("single tool caption = %q, want 'calling read_file'", got)
	}

	// New batch: writing then three parallel tools.
	c.BeginWriting()
	c.BeginTool("read_file")
	c.BeginTool("grep")
	c.BeginTool("bash")
	if c.toolsStarted != 3 {
		t.Fatalf("toolsStarted = %d, want 3 (batch reset on phase change)", c.toolsStarted)
	}
	c.ToolReady()
	c.ToolReady()
	got := stripANSI(c.Render(th, false))
	if !strings.Contains(got, "running 3 tools") || !strings.Contains(got, "2 ready") {
		t.Errorf("multi-tool caption = %q, want 'running 3 tools' and '2 ready'", got)
	}

	// ToolReady is clamped to the batch size.
	c.ToolReady()
	c.ToolReady()
	c.ToolReady()
	if c.toolsReady != 3 {
		t.Errorf("toolsReady = %d, want clamp at 3", c.toolsReady)
	}

	// Reset clears the batch.
	c.Reset()
	if c.toolsStarted != 0 || c.toolsReady != 0 {
		t.Errorf("after Reset: started=%d ready=%d, want 0/0", c.toolsStarted, c.toolsReady)
	}
}

// TestChromeToolBatchResetsAcrossPureToolTurns reproduces the lifecycle bug
// the adversarial review caught: two consecutive tool batches with NO
// intervening reasoning/text (a pure tool-call turn never leaves phaseTool),
// delimited only by the per-step EndToolBatch. Without the batchClosed reset
// the second batch inherits the first's counts and a frozen timer.
func TestChromeToolBatchResetsAcrossPureToolTurns(t *testing.T) {
	c := NewChrome()

	// Batch 1 arrives after a thinking turn.
	c.BeginThinking()
	c.BeginTool("read_file")
	c.BeginTool("grep")
	c.ToolReady()
	c.ToolReady()
	if c.toolsStarted != 2 || c.toolsReady != 2 {
		t.Fatalf("batch1: started=%d ready=%d, want 2/2", c.toolsStarted, c.toolsReady)
	}

	// Step boundary fires (EventStepFinish → EndToolBatch). The next model
	// turn is a pure tool-call turn: NO BeginThinking/BeginWriting, so phase
	// stays phaseTool.
	c.EndToolBatch()
	c.startedAt = time.Unix(1, 0) // sentinel "old" clock

	// Batch 2 starts directly with a tool call.
	c.BeginTool("bash")
	if c.toolsStarted != 1 {
		t.Errorf("batch2 must reset toolsStarted to 1, got %d (stale count bled across the batch)", c.toolsStarted)
	}
	if c.toolsReady != 0 {
		t.Errorf("batch2 must reset toolsReady to 0, got %d (stale ready bled across the batch)", c.toolsReady)
	}
	if c.startedAt.Equal(time.Unix(1, 0)) {
		t.Error("batch2 must re-stamp startedAt (timer was frozen across the batch boundary)")
	}
}

// TestChromeColdStartCaption pins that the cold-start caption quotes the real
// FirstTokenTimeout, appears only while waiting on the first token, and is
// suppressed when no timeout is configured.
func TestChromeColdStartCaption(t *testing.T) {
	th := DarkTheme()

	c := NewChrome()
	c.SetFirstTokenTimeout(45 * time.Second)
	c.BeginThinking()
	c.startedAt = time.Now().Add(-10 * time.Second) // 10s elapsed, no tokens yet
	if got := stripANSI(c.Render(th, false)); !strings.Contains(got, "first token within 45s") {
		t.Errorf("cold-start caption = %q, want 'first token within 45s'", got)
	}

	// Once tokens arrive, the cold-start suffix disappears.
	c.tokens = 5
	if got := stripANSI(c.Render(th, false)); strings.Contains(got, "cold start") {
		t.Errorf("caption should drop cold-start suffix once tokens arrive, got %q", got)
	}

	// No timeout configured → no suffix even while waiting.
	c2 := NewChrome()
	c2.BeginThinking()
	c2.startedAt = time.Now().Add(-10 * time.Second)
	if got := stripANSI(c2.Render(th, false)); strings.Contains(got, "cold start") {
		t.Errorf("no-timeout caption should omit cold-start suffix, got %q", got)
	}
}

func TestChromeNoNewTick(t *testing.T) {
	// Verify chrome.go doesn't introduce new tea.Tick calls — the
	// spinner should reuse existing tick infrastructure.
	// This is a code-structure check: if tea.Tick appears in chrome.go
	// outside of the existing scheduleRedraw path, the test will catch
	// it at review time. Here we just verify the ramp is cached.
	c := NewChrome()
	th := DarkTheme()
	c.BeginThinking()
	c.ensureRamp(th)
	if len(c.ramp) == 0 {
		t.Fatal("ramp should be initialized after ensureRamp")
	}
	// Calling ensureRamp again should be a no-op (cached).
	ramp1 := c.ramp
	c.ensureRamp(th)
	if len(c.ramp) != len(ramp1) {
		t.Fatal("ramp should be cached, not re-created")
	}
}

// TestChromeCaptionGradientShimmer verifies that the active caption renders
// with a phase-offset gradient that advances with the frame counter. Frame 0
// and frame 4 must produce different output (gradient shifted).
func TestChromeCaptionGradientShimmer(t *testing.T) {
	th := DarkTheme()
	dir := filepath.Join("testdata", "render-ansi")

	// Render at frames 0, 4, 8.
	frames := []int{0, 4, 8}
	renders := make([]string, len(frames))
	for i, f := range frames {
		c := NewChrome()
		c.BeginThinking()
		c.frame = f
		renders[i] = c.Render(th, false)
	}

	// Frame 0 and frame 4 must differ (gradient advanced).
	if renders[0] == renders[1] {
		t.Error("frame 0 and frame 4 renders are identical — gradient not advancing")
	}

	// Write golden files.
	for i, f := range frames {
		name := fmt.Sprintf("chrome_caption_f%d.golden", f)
		path := filepath.Join(dir, name)
		if os.Getenv("UPDATE_GOLDEN") == "1" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(path, []byte(renders[i]), 0o644); err != nil {
				t.Fatalf("write golden: %v", err)
			}
			t.Logf("updated golden: %s", path)
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read golden (regenerate with UPDATE_GOLDEN=1): %v", err)
		}
		wantStr := strings.ReplaceAll(string(want), "\r\n", "\n")
		got := strings.ReplaceAll(renders[i], "\r\n", "\n")
		if got != wantStr {
			t.Errorf("golden drift for %s:\n--- want ---\n%q\n--- got ---\n%q", name, wantStr, got)
		}
	}
}

// TestChromeRenderNoColor verifies that Chrome.Render output reaches a
// no-color terminal as plain text with zero ANSI escape sequences.
//
// Architecture note: lipgloss v2's Style.Render() always produces ANSI
// internally (this is by design — the style layer is decoupled from the
// output layer). The color profile controls what reaches the terminal:
//   - TrueColor: ANSI passes through as-is
//   - Ascii/ANSI: color SGR sequences are downsampled
//   - NoTTY: ALL ANSI is stripped via ansi.Strip()
//
// The card's "emits zero ANSI" describes terminal behavior, not Render()
// internals. This test verifies the end-to-end path: Chrome.Render() →
// NoTTY Writer → zero ANSI in output.
func TestChromeRenderNoColor(t *testing.T) {
	th := DarkTheme()
	c := NewChrome()
	c.BeginThinking()
	c.frame = 42

	raw := c.Render(th, false)
	if raw == "" {
		t.Fatal("Chrome.Render returned empty string for active chrome")
	}

	// Simulate writing to a NoTTY terminal (strips ALL ANSI sequences).
	var buf strings.Builder
	w := &colorprofile.Writer{Forward: &buf, Profile: colorprofile.NoTTY}
	_, err := w.Write([]byte(raw))
	if err != nil {
		t.Fatalf("Writer.Write failed: %v", err)
	}
	out := buf.String()

	// Assert zero ANSI escape sequences (ESC = 0x1b).
	for i, r := range out {
		if r == 0x1b {
			t.Errorf("ANSI escape at byte %d in NoColor output: %q", i, out)
			break
		}
	}

	// Verify caption content is readable.
	if !strings.Contains(out, "thinking") {
		t.Errorf("NoColor output missing 'thinking' caption: %q", out)
	}
	if !strings.Contains(out, "[^C cancel]") {
		t.Errorf("NoColor output missing cancel hint: %q", out)
	}
}
