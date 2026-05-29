package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// TestCompactionClearsFileTracker proves the T3.2 invalidate-on-fold wiring:
// after maybeCompact folds the transcript, the shared read-before-write tracker
// is cleared, so a file read before the fold must be re-read before editing.
func TestCompactionClearsFileTracker(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.go")
	if err := os.WriteFile(p, []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := tools.New()
	tools.RegisterBuiltins(reg, 1<<20, 1<<20, dir)
	ft := reg.FileTracker()
	if ft == nil {
		t.Fatal("registry has no FileTracker")
	}
	ft.RecordRead(p)
	if err := ft.CheckFresh(p); err != nil {
		t.Fatalf("precondition: file should be fresh after read, got %v", err)
	}

	a := New(nil, reg, nil, "m")
	a.DisableSemanticCompaction = true
	a.CompactionCfg = CompactionConfig{PreserveRecentMessages: 2, AutoCompactInputTokens: 100}
	for i := 0; i < 20; i++ {
		a.Messages = append(a.Messages, llm.Message{
			Role:   "user",
			Blocks: []llm.ContentBlock{llm.TextBlock{Text: strings.Repeat("x", 400)}},
		})
	}
	before := len(a.Messages)
	a.maybeCompact(context.Background())
	if len(a.Messages) >= before {
		t.Fatalf("expected compaction to shrink messages; before=%d after=%d", before, len(a.Messages))
	}

	// The fold cleared the tracker: editing now requires a fresh re-read.
	if err := ft.CheckFresh(p); err == nil {
		t.Error("expected the file tracker to be cleared on fold (re-read required), but CheckFresh passed")
	}
}
