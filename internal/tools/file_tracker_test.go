package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func ftArgs(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func TestFileTrackerNilSafe(t *testing.T) {
	var ft *FileTracker // nil
	if err := ft.CheckFresh("/nope"); err != nil {
		t.Errorf("nil tracker CheckFresh = %v, want nil (guard disabled)", err)
	}
	// Must not panic.
	ft.RecordRead("/x")
	ft.RecordWrite("/x")
	ft.Clear()
}

func TestFileTrackerCheckFreshLifecycle(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(p, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	ft := NewFileTracker()

	// Existing but never read → error.
	if err := ft.CheckFresh(p); err == nil {
		t.Error("expected error for an existing-but-unread file")
	}
	// Non-existent → nil (a legitimate create).
	if err := ft.CheckFresh(filepath.Join(dir, "new.txt")); err != nil {
		t.Errorf("non-existent path should pass (create), got %v", err)
	}
	// Read, then check → fresh.
	ft.RecordRead(p)
	if err := ft.CheckFresh(p); err != nil {
		t.Errorf("just-read file should be fresh, got %v", err)
	}
	// Size change → stale.
	if err := os.WriteFile(p, []byte("hello world, longer now"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ft.CheckFresh(p); err == nil {
		t.Error("expected stale after a size change")
	}
}

func TestFileTrackerDetectsMtimeOnlyChange(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(p, []byte("same-size"), 0o644); err != nil {
		t.Fatal(err)
	}
	ft := NewFileTracker()
	ft.RecordRead(p)

	// Rewrite identical-length content, then bump mtime explicitly so the test
	// is deterministic regardless of filesystem mtime resolution.
	if err := os.WriteFile(p, []byte("same-size"), 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(3 * time.Second)
	if err := os.Chtimes(p, future, future); err != nil {
		t.Fatal(err)
	}
	if err := ft.CheckFresh(p); err == nil {
		t.Error("expected stale after an mtime-only change")
	}
}

func TestFileTrackerRecordWriteAndClear(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(p, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	ft := NewFileTracker()
	ft.RecordRead(p)

	// A write changes the file; RecordWrite must re-stamp so it stays fresh.
	if err := os.WriteFile(p, []byte("v2-longer"), 0o644); err != nil {
		t.Fatal(err)
	}
	ft.RecordWrite(p)
	if err := ft.CheckFresh(p); err != nil {
		t.Errorf("RecordWrite should refresh the stamp, got %v", err)
	}

	// Clear drops records → unread again.
	ft.Clear()
	if err := ft.CheckFresh(p); err == nil {
		t.Error("expected unread after Clear()")
	}
}

// --- tools-level integration: the guard fires through the actual file tools ---

func TestEditFileFreshnessGuard(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	p := filepath.Join(dir, "f.go")
	if err := os.WriteFile(p, []byte("package x\n\nvar A = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ft := NewFileTracker()
	rf := ReadFile{CWD: dir, Tracker: ft}
	ef := EditFile{CWD: dir, Tracker: ft}

	// Edit before any read → rejected.
	res, _ := ef.Execute(ctx, ftArgs(map[string]any{"path": p, "old_string": "var A = 1", "new_string": "var A = 2"}))
	if !res.IsError {
		t.Fatal("edit before read should be rejected")
	}

	// Read (stamps via the shared tracker), then edit → allowed.
	if r, _ := rf.Execute(ctx, ftArgs(map[string]any{"path": p})); r.IsError {
		t.Fatalf("read failed: %s", r.Content)
	}
	res, _ = ef.Execute(ctx, ftArgs(map[string]any{"path": p, "old_string": "var A = 1", "new_string": "var A = 2"}))
	if res.IsError {
		t.Fatalf("edit after read should succeed, got: %s", res.Content)
	}

	// An out-of-band change (different size) → next edit rejected until re-read.
	if err := os.WriteFile(p, []byte("package x\n\nvar A = 2\nvar B = 3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, _ = ef.Execute(ctx, ftArgs(map[string]any{"path": p, "old_string": "var A = 2", "new_string": "var A = 9"}))
	if !res.IsError {
		t.Fatal("edit after an out-of-band change should be rejected")
	}
}

func TestWriteFileFreshnessGuard(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	ft := NewFileTracker()
	wf := WriteFile{CWD: dir, Tracker: ft}

	// New file (absent) → allowed even without a prior read.
	newp := filepath.Join(dir, "new.txt")
	if res, _ := wf.Execute(ctx, ftArgs(map[string]any{"path": newp, "content": "hi"})); res.IsError {
		t.Fatalf("creating a new file should be allowed, got: %s", res.Content)
	}
	// The create stamped it, so overwriting it again is allowed.
	if res, _ := wf.Execute(ctx, ftArgs(map[string]any{"path": newp, "content": "hi again"})); res.IsError {
		t.Fatalf("overwriting our own just-written file should be allowed, got: %s", res.Content)
	}

	// An existing file never read → overwrite rejected.
	exist := filepath.Join(dir, "exist.txt")
	if err := os.WriteFile(exist, []byte("precious"), 0o644); err != nil {
		t.Fatal(err)
	}
	if res, _ := wf.Execute(ctx, ftArgs(map[string]any{"path": exist, "content": "clobber"})); !res.IsError {
		t.Fatal("overwriting an existing unread file should be rejected")
	}
}

// TestApplyPatchExemptFromFreshnessGuard pins the deliberate exemption: an
// apply_patch update to a file that was never read is NOT rejected (its hunk
// context lines self-validate), and the write re-stamps the tracker so a
// follow-up edit_file is allowed without a re-read (T3.2).
func TestApplyPatchExemptFromFreshnessGuard(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	orig := filepath.Join(dir, "f.go")
	if err := os.WriteFile(orig, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ft := NewFileTracker()
	tool := NewApplyPatchTool(dir, 1<<20)
	tool.tracker = ft

	res, err := tool.Execute(ctx, mustPatchArgs(`*** Begin Patch
*** Update File: f.go
@@
 a
-b
+B
 c
*** End Patch`))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("apply_patch on an unread file must NOT be gated, got: %s", res.Content)
	}

	// The patch re-stamped the file, so a follow-up edit_file is fresh.
	ef := EditFile{CWD: dir, Tracker: ft}
	r2, _ := ef.Execute(ctx, ftArgs(map[string]any{"path": orig, "old_string": "B", "new_string": "BB"}))
	if r2.IsError {
		t.Fatalf("edit after apply_patch (re-stamped) should be allowed, got: %s", r2.Content)
	}
}

// TestRegistryClonesPropagateFileTracker pins the adversarial HIGH-1 fix: every
// registry-cloning method shares the SAME tracker instance, so a subagent built
// from a Subset/TierTools/CloneForCWD still has a working FileTracker() — and
// thus a working invalidate-on-fold Clear().
func TestRegistryClonesPropagateFileTracker(t *testing.T) {
	dir := t.TempDir()
	r := New()
	RegisterBuiltins(r, 1<<20, 1<<20, dir)
	parent := r.FileTracker()
	if parent == nil {
		t.Fatal("parent registry has no FileTracker")
	}
	if got := r.Subset([]string{"read_file", "edit_file"}).FileTracker(); got != parent {
		t.Errorf("Subset dropped the FileTracker (got %p, want %p)", got, parent)
	}
	if got := r.CloneForCWD(dir).FileTracker(); got != parent {
		t.Errorf("CloneForCWD dropped the FileTracker (got %p, want %p)", got, parent)
	}
	if got := r.TierTools().FileTracker(); got != parent {
		t.Errorf("TierTools dropped the FileTracker (got %p, want %p)", got, parent)
	}
}

// TestReadTooLargeStampsSoWriteIsAllowed pins the adversarial HIGH-2 fix: a file
// that exceeds the read cap is still stamped, so a subsequent write_file isn't
// dead-ended by a false "never read" rejection.
func TestReadTooLargeStampsSoWriteIsAllowed(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	p := filepath.Join(dir, "big.txt")
	if err := os.WriteFile(p, []byte(strings.Repeat("x", 4096)), 0o644); err != nil {
		t.Fatal(err)
	}
	ft := NewFileTracker()
	rf := ReadFile{CWD: dir, MaxBytes: 100, Tracker: ft} // cap below file size
	wf := WriteFile{CWD: dir, Tracker: ft}

	// read_file refuses (too large) but must still stamp.
	res, _ := rf.Execute(ctx, ftArgs(map[string]any{"path": p}))
	if !res.IsError || !strings.Contains(res.Content, "too large") {
		t.Fatalf("expected a too-large error, got: %#v", res)
	}
	// The stamp means an overwrite is NOT dead-ended as "never read".
	res, _ = wf.Execute(ctx, ftArgs(map[string]any{"path": p, "content": "small now"}))
	if res.IsError {
		t.Fatalf("write after a too-large read should be allowed (stamped), got: %s", res.Content)
	}
}

func TestRegisterBuiltinsSharesFileTracker(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	p := filepath.Join(dir, "f.go")
	if err := os.WriteFile(p, []byte("package x\n\nvar A = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := New()
	RegisterBuiltins(r, 1<<20, 1<<20, dir)
	if r.FileTracker() == nil {
		t.Fatal("RegisterBuiltins must install a FileTracker on the registry")
	}

	read, okR := r.Get("read_file")
	edit, okE := r.Get("edit_file")
	if !okR || !okE {
		t.Fatal("read_file/edit_file not registered")
	}

	// Edit before read → rejected, proving the registered edit_file carries the
	// shared tracker.
	res, _ := edit.Execute(ctx, ftArgs(map[string]any{"path": p, "old_string": "var A = 1", "new_string": "var A = 2"}))
	if !res.IsError {
		t.Fatal("edit before read should be rejected through the registry tools")
	}
	// Read through the registry's read_file, then edit → allowed (shared tracker).
	if r2, _ := read.Execute(ctx, ftArgs(map[string]any{"path": p})); r2.IsError {
		t.Fatalf("read failed: %s", r2.Content)
	}
	res, _ = edit.Execute(ctx, ftArgs(map[string]any{"path": p, "old_string": "var A = 1", "new_string": "var A = 2"}))
	if res.IsError {
		t.Fatalf("edit after read should succeed through the registry tools, got: %s", res.Content)
	}
}
