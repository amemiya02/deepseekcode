package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestAgentDefsResult exercises the doctor agent-def diagnostic: it surfaces a
// silently-skipped (oversized) def and an out-of-range temperature field.
func TestAgentDefsResult(t *testing.T) {
	cwd := t.TempDir()
	dir := filepath.Join(cwd, ".deepseek", "agent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("good.md", "---\ndescription: ok\ntemperature: 0.5\n---\nbody")
	write("hot.md", "---\ndescription: hot\ntemperature: 5.0\n---\nbody")
	// >64 KiB → silently skipped by agents.Load's size cap.
	write("huge.md", "---\ndescription: huge\n---\n"+strings.Repeat("x", 65*1024))

	// home="" so only the cwd tree is scanned (no host-dir pollution).
	res := agentDefsResult(cwd, "")

	if res.Status != "warn" {
		t.Fatalf("status = %q, want warn; detail=%q", res.Status, res.Detail)
	}
	if !strings.Contains(res.Detail, "2 loaded") {
		t.Errorf("detail should report 2 loaded, got %q", res.Detail)
	}
	if !strings.Contains(res.Detail, "skipped") {
		t.Errorf("detail should flag the oversized skipped def, got %q", res.Detail)
	}
	if !strings.Contains(res.Detail, "hot: temperature 5.00 out of [0,2]") {
		t.Errorf("detail should flag the out-of-range temperature, got %q", res.Detail)
	}
}

// TestAgentDefsResultClean confirms a tidy cwd reports ok.
func TestAgentDefsResultClean(t *testing.T) {
	cwd := t.TempDir()
	dir := filepath.Join(cwd, ".deepseek", "agent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scout.md"), []byte("---\ndescription: scout\ntemperature: 0.2\ntop_p: 0.9\n---\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := agentDefsResult(cwd, "")
	if res.Status != "ok" {
		t.Fatalf("status = %q, want ok; detail=%q", res.Status, res.Detail)
	}
	if !strings.Contains(res.Detail, "1 loaded") {
		t.Errorf("detail = %q, want '1 loaded'", res.Detail)
	}
}

// TestREADMEDocLinksResolve fails when a README links a repo-relative markdown
// file that does not exist — catching doc rot like a renamed/deleted doc or a
// typo'd path (e.g. the docs/MODEL_COMPATIBILITY.md link added in T7.4). The
// PARITY.md scenario linkage is separately enforced by internal/llm's
// TestParityConsistency, so this is non-redundant.
func TestREADMEDocLinksResolve(t *testing.T) {
	repoRoot := filepath.Join("..", "..") // cmd/dsc -> repo root
	mdLink := regexp.MustCompile(`\]\(([^)]+)\)`)
	for _, name := range []string{"README.md", "README.zh-CN.md"} {
		data, err := os.ReadFile(filepath.Join(repoRoot, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, m := range mdLink.FindAllStringSubmatch(string(data), -1) {
			target := m[1]
			switch {
			case strings.HasPrefix(target, "http://"), strings.HasPrefix(target, "https://"),
				strings.HasPrefix(target, "#"), strings.HasPrefix(target, "mailto:"):
				continue
			}
			if i := strings.IndexByte(target, '#'); i >= 0 {
				target = target[:i] // strip in-page anchor
			}
			if !strings.HasSuffix(target, ".md") {
				continue // doc-reference linkage only
			}
			if _, err := os.Stat(filepath.Join(repoRoot, target)); err != nil {
				t.Errorf("%s links %q which does not resolve: %v", name, m[1], err)
			}
		}
	}
}
