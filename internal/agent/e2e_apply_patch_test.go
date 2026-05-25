package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/session"
	"github.com/amemiya02/deepseekcode/internal/snapshots"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// TestE2EApplyPatchAffectedPaths verifies that AffectedPathsFor returns
// all target paths from an apply_patch call touching 2 files.
func TestE2EApplyPatchAffectedPaths(t *testing.T) {
	cwd := t.TempDir()
	reg := tools.New()
	tools.RegisterBuiltins(reg, 1024, 1024, cwd)

	patchText := "*** Begin Patch\n*** Add File: a.txt\n+hello\n*** Add File: b.txt\n+world\n*** End Patch"
	args := map[string]any{"patchText": patchText}
	argsJSON, _ := json.Marshal(args)

	call := llm.ToolCall{
		ID:   "c1",
		Type: "function",
		Function: llm.ToolCallFunc{
			Name:      "apply_patch",
			Arguments: string(argsJSON),
		},
	}

	paths := AffectedPathsFor(reg, call)
	if len(paths) != 2 {
		t.Fatalf("AffectedPathsFor returned %d paths, want 2: %v", len(paths), paths)
	}
	names := map[string]bool{}
	for _, p := range paths {
		names[filepath.Base(p)] = true
	}
	if !names["a.txt"] || !names["b.txt"] {
		t.Errorf("expected a.txt and b.txt in paths, got %v", paths)
	}
}

// TestE2EApplyPatchSnapshotUndo verifies that apply_patch triggers
// pre-tool snapshots and that undo restores the affected files.
func TestE2EApplyPatchSnapshotUndo(t *testing.T) {
	cwd := t.TempDir()
	f1 := filepath.Join(cwd, "f1.txt")
	if err := os.WriteFile(f1, []byte("original1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f2 := filepath.Join(cwd, "f2.txt")
	if err := os.WriteFile(f2, []byte("original2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	patchText := "*** Begin Patch\n*** Update File: f1.txt\n@@\n-original1\n+updated1\n*** Update File: f2.txt\n@@\n-original2\n+updated2\n*** End Patch"
	args := map[string]any{"patchText": patchText}
	argsJSON, _ := json.Marshal(args)
	argsStr, _ := json.Marshal(string(argsJSON))

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		switch n {
		case 1:
			emitSSE(w,
				`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"apply_patch","arguments":`+string(argsStr)+`}}]}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`,
			)
		case 2:
			emitSSE(w,
				`{"choices":[{"index":0,"delta":{"content":"done"}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":15,"completion_tokens":1,"total_tokens":16}}`,
			)
		default:
			http.Error(w, "unexpected extra call", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	dbPath := filepath.Join(cwd, ".deepseek", "sessions", "test.db")
	os.MkdirAll(filepath.Dir(dbPath), 0o755)
	store, err := session.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	sess, err := store.NewSession(context.Background(), cwd, "test-model", false)
	if err != nil {
		t.Fatal(err)
	}
	sessID := sess.ID
	if err != nil {
		t.Fatal(err)
	}

	snaps := snapshots.New(filepath.Join(cwd, ".deepseek", "snapshots"))
	persister := session.NewPersister(store, snaps, sessID)

	reg := tools.New()
	tools.RegisterBuiltins(reg, 1024, 1024, cwd)

	a := New(llm.NewClient("k", srv.URL), reg, permissions.New(permissions.ModeYolo, cwd, nil, nil, nil), "test-model")
	a.Persister = persister
	a.StopWhen = []StopCondition{MaxSteps(2)}

	_, err = a.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Verify files were updated.
	data1, _ := os.ReadFile(f1)
	if string(data1) != "updated1\n" {
		t.Errorf("f1.txt after patch = %q, want %q", string(data1), "updated1\n")
	}
	data2, _ := os.ReadFile(f2)
	if string(data2) != "updated2\n" {
		t.Errorf("f2.txt after patch = %q, want %q", string(data2), "updated2\n")
	}

	// Undo should restore both files.
	restored, err := persister.Undo(1)
	if err != nil {
		t.Fatalf("Undo: %v", err)
	}
	if restored < 2 {
		t.Errorf("Undo restored %d files, want >= 2", restored)
	}

	data1, _ = os.ReadFile(f1)
	if string(data1) != "original1\n" {
		t.Errorf("f1.txt after undo = %q, want %q", string(data1), "original1\n")
	}
	data2, _ = os.ReadFile(f2)
	if string(data2) != "original2\n" {
		t.Errorf("f2.txt after undo = %q, want %q", string(data2), "original2\n")
	}
}
