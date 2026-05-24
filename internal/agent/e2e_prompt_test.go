package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/prompt"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// TestE2EPromptCacheStable verifies the cache-stability invariant
// in a realistic two-run sequence:
//   - The bytes BEFORE DynamicContextBoundary are byte-identical
//     across turns (cache prefix preserved).
//   - The bytes AFTER the boundary differ once the working tree
//     changes (per-turn dynamic context refreshed).
//
// The fake LLM server returns a trivial stop response so each Run
// goes through exactly one step — enough to fire refreshGitContext.
func TestE2EPromptCacheStable(t *testing.T) {
	dir := t.TempDir()
	mustGit(t, dir, "init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "seed.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	mustGit(t, dir, "add", "seed.go")
	mustGit(t, dir, "-c", "user.name=t", "-c", "user.email=t@e", "commit", "-q", "-m", "init")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	mk := func() *Agent {
		a := New(llm.NewClient("k", srv.URL), tools.New(),
			permissions.New(permissions.ModeYolo, "", nil, nil), "m")
		project := prompt.DiscoverProjectContext(dir)
		a.PromptBuilder = &prompt.SystemPromptBuilder{
			StaticBase: prompt.BasePromptV1,
			Project:    &project,
		}
		a.StopWhen = []StopCondition{MaxSteps(1)}
		return a
	}

	// Run 1: clean working tree (after the seeded commit). The git
	// reader caches for 5s by default — give a fresh Reader to each
	// agent so each Run hits git anew.
	a1 := mk()
	if _, err := a1.Run(context.Background(), "first"); err != nil {
		t.Fatalf("run 1: %v", err)
	}
	prompt1 := a1.System

	// Working-tree change between runs.
	if err := os.WriteFile(filepath.Join(dir, "new.go"), []byte("package y\n"), 0o644); err != nil {
		t.Fatalf("write new.go: %v", err)
	}
	// Tiny sleep keeps the cached snapshot in the second agent's
	// reader fresh enough to see the new file (defensive — fresh
	// reader instance per mk(), no shared cache).
	time.Sleep(10 * time.Millisecond)

	a2 := mk()
	if _, err := a2.Run(context.Background(), "second"); err != nil {
		t.Fatalf("run 2: %v", err)
	}
	prompt2 := a2.System

	i1 := strings.Index(prompt1, prompt.DynamicContextBoundary)
	i2 := strings.Index(prompt2, prompt.DynamicContextBoundary)
	if i1 < 0 || i2 < 0 {
		t.Fatalf("boundary missing: i1=%d i2=%d", i1, i2)
	}
	if prompt1[:i1] != prompt2[:i2] {
		t.Errorf("static prefix drifted across runs")
	}
	if prompt1[i1:] == prompt2[i2:] {
		t.Errorf("dynamic suffix should differ after working-tree change")
	}
	if !strings.Contains(prompt2, "new.go") {
		t.Errorf("run 2 prompt should mention new.go in git status; got tail: %q", trimMid(prompt2, 400))
	}
}
