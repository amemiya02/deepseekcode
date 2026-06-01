package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// goldenItem pairs a fixture name with a chat item whose rendering is
// width-sensitive (header truncation, body folding, word wrapping, diff
// routing). Pinning these across widths catches layout regressions that a
// content-hash render cache cannot — the cache keys on width, so a width-
// dependent layout bug reproduces identically on every cache hit.
type goldenItem struct {
	name string
	item chatItem
}

func goldenItems() []goldenItem {
	longDiff := "diff --git a/internal/agent/agent.go b/internal/agent/agent.go\n" +
		"index abc1234..def5678 100644\n" +
		"--- a/internal/agent/agent.go\n" +
		"+++ b/internal/agent/agent.go\n" +
		"@@ -10,7 +10,9 @@ func (a *Agent) runStep() {\n" +
		"-\told := a.compute()\n" +
		"+\tnew := a.compute()\n" +
		"+\ta.foldCacheUsage(new)\n" +
		" \treturn nil\n"

	// Intra-line diff: adjacent del/add with word-level changes.
	// Only "old"/"new" differ; " := a.compute()" is shared.
	intraLineDiff := "diff --git a/foo.go b/foo.go\n" +
		"index abc1234..def5678 100644\n" +
		"--- a/foo.go\n" +
		"+++ b/foo.go\n" +
		"@@ -1,1 +1,1 @@\n" +
		"-old := a.compute()\n" +
		"+new := a.compute()\n"

	// No intra-line change: del/add have completely different content.
	// No word-level match → whole-line band, no emphasis spans.
	noIntraLineDiff := "diff --git a/bar.go b/bar.go\n" +
		"index abc1234..def5678 100644\n" +
		"--- a/bar.go\n" +
		"+++ b/bar.go\n" +
		"@@ -1,2 +1,2 @@\n" +
		"-removed line entirely\n" +
		"+completely different content\n"

	var manyLines strings.Builder
	for i := 1; i <= 15; i++ {
		fmt.Fprintf(&manyLines, "line %d of command output\n", i)
	}

	return []goldenItem{
		{"tool_call_long", chatItem{
			kind: itemToolCall, tool: "bash",
			args:     `{"command":"git log --oneline --graph --decorate --all -n 50"}`,
			duration: 5 * time.Millisecond,
		}},
		{"tool_result_diff", chatItem{
			kind: itemToolResult, tool: "bash", args: `{"command":"git diff"}`,
			result: tools.Result{Content: longDiff}, expanded: true, duration: 12 * time.Millisecond,
		}},
		{"tool_result_folded", chatItem{
			kind: itemToolResult, tool: "bash", args: `{"command":"ls -la /very/long/path/that/forces/header/truncation"}`,
			result: tools.Result{Content: manyLines.String()}, duration: 3 * time.Millisecond,
		}},
		{"reasoning_expanded", chatItem{
			kind: itemReasoning,
			reasoning: "I should read the agent loop first, then trace how the prefix " +
				"epoch fingerprint is computed and whether a body message change can move it.",
			duration: 2500 * time.Millisecond, tokens: 42,
		}},
		{"tool_result_error", chatItem{
			kind: itemToolResult, tool: "bash", args: `{"command":"rm -rf /"}`,
			result:   tools.Result{Content: "permission denied", IsError: true},
			duration: 50 * time.Millisecond,
		}},
		{"diff_intra_line", chatItem{
			kind: itemToolResult, tool: "bash", args: `{"command":"git diff"}`,
			result: tools.Result{Content: intraLineDiff}, expanded: true, duration: 8 * time.Millisecond,
		}},
		{"diff_no_intra_line", chatItem{
			kind: itemToolResult, tool: "bash", args: `{"command":"git diff"}`,
			result: tools.Result{Content: noIntraLineDiff}, expanded: true, duration: 4 * time.Millisecond,
		}},
	}
}

// TestRenderGoldenPerWidth pins tool-call/result/reasoning rendering across
// narrow/default/wide widths. Regenerate after an intentional layout change:
//
//	UPDATE_GOLDEN=1 go test ./internal/tui/ -run TestRenderGoldenPerWidth
func TestRenderGoldenPerWidth(t *testing.T) {
	widths := []int{40, 80, 120}
	th := DarkTheme()
	dir := filepath.Join("testdata", "render")

	for _, c := range goldenItems() {
		for _, w := range widths {
			name := fmt.Sprintf("%s_w%d", c.name, w)
			t.Run(name, func(t *testing.T) {
				got := normalizeGolden(c.item.render(th, w))
				path := filepath.Join(dir, name+".golden")

				if os.Getenv("UPDATE_GOLDEN") == "1" {
					if err := os.MkdirAll(dir, 0o755); err != nil {
						t.Fatalf("mkdir: %v", err)
					}
					if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
						t.Fatalf("write golden: %v", err)
					}
					t.Logf("updated golden: %s", path)
					return
				}

				want, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("read golden (regenerate with UPDATE_GOLDEN=1 go test ./internal/tui/ -run TestRenderGoldenPerWidth): %v", err)
				}
				// Normalize CRLF → LF so golden files checked out with
				// core.autocrlf=true (Windows CI) still compare correctly.
				wantStr := strings.ReplaceAll(string(want), "\r\n", "\n")
				if got != wantStr {
					t.Errorf("golden drift for %s:\n--- want ---\n%s\n--- got ---\n%s", name, string(want), got)
				}
			})
		}
	}
}

// normalizeGolden strips ANSI and trailing whitespace so the golden captures
// layout (truncation/wrapping/folding), not color codes that vary by terminal
// color profile. This keeps the fixtures portable and review-friendly.
func normalizeGolden(s string) string {
	s = stripANSI(s)
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	return strings.Join(lines, "\n")
}

// TestGoldenWidth40Constraint measures every line of the width-40 layout
// goldens with lipgloss.Width and asserts ≤ 40 display cols. This catches
// CJK/emoji width bugs and right-aligned duration overflow.
func TestGoldenWidth40Constraint(t *testing.T) {
	dir := filepath.Join("testdata", "render")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, "_w40.golden") {
			continue
		}
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}
			lines := strings.Split(string(raw), "\n")
			for i, line := range lines {
				// Measure with lipgloss.Width (handles ANSI + CJK).
				w := lipgloss.Width(line)
				if w > 40 {
					t.Errorf("line %d: lipgloss.Width=%d > 40: %q", i+1, w, line)
				}
			}
		})
	}
}

// normalizeGoldenKeepANSI trims trailing per-line whitespace but PRESERVES
// ANSI escapes so color/gradient regressions are caught. Used by
// TestRenderGoldenANSI for color-sensitive golden files.
func normalizeGoldenKeepANSI(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	return strings.Join(lines, "\n")
}

// TestRenderGoldenANSI pins tool-call/result/reasoning rendering with ANSI
// escapes preserved (color-sensitive). Regenerate after an intentional color
// change:
//
//	UPDATE_GOLDEN=1 go test ./internal/tui/ -run TestRenderGoldenANSI
func TestRenderGoldenANSI(t *testing.T) {
	widths := []int{40, 80, 120}
	th := DarkTheme()
	dir := filepath.Join("testdata", "render-ansi")

	for _, c := range goldenItems() {
		for _, w := range widths {
			name := fmt.Sprintf("%s_w%d", c.name, w)
			t.Run(name, func(t *testing.T) {
				got := normalizeGoldenKeepANSI(c.item.render(th, w))
				path := filepath.Join(dir, name+".golden")

				if os.Getenv("UPDATE_GOLDEN") == "1" {
					if err := os.MkdirAll(dir, 0o755); err != nil {
						t.Fatalf("mkdir: %v", err)
					}
					if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
						t.Fatalf("write golden: %v", err)
					}
					t.Logf("updated ANSI golden: %s", path)
					return
				}

				want, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("read golden (regenerate with UPDATE_GOLDEN=1 go test ./internal/tui/ -run TestRenderGoldenANSI): %v", err)
				}
				wantStr := strings.ReplaceAll(string(want), "\r\n", "\n")
				if got != wantStr {
					t.Errorf("ANSI golden drift for %s:\n--- want ---\n%s\n--- got ---\n%s", name, string(want), got)
				}
			})
		}
	}
}
