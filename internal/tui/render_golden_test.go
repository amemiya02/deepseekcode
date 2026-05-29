package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

	var manyLines strings.Builder
	for i := 1; i <= 15; i++ {
		fmt.Fprintf(&manyLines, "line %d of command output\n", i)
	}

	return []goldenItem{
		{"tool_call_long", chatItem{
			kind: itemToolCall, tool: "bash",
			args: `{"command":"git log --oneline --graph --decorate --all -n 50"}`,
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
				if got != string(want) {
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
