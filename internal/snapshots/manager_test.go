package snapshots

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotRoundTrip(t *testing.T) {
	repoDir := t.TempDir()
	snapDir := t.TempDir()
	mgr := New(snapDir)

	target := filepath.Join(repoDir, "hello.txt")
	if err := os.WriteFile(target, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Take a snapshot, then modify the file.
	n, err := mgr.Take("sess1", 0, []string{target})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("Take returned %d, want 1", n)
	}
	if err := os.WriteFile(target, []byte("MUTATED"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Undo should restore.
	restored, err := mgr.Undo("sess1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if restored != 1 {
		t.Fatalf("Undo restored %d, want 1", restored)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original" {
		t.Fatalf("got %q, want %q", got, "original")
	}
}

func TestSnapshotAbsentTombstone(t *testing.T) {
	repoDir := t.TempDir()
	snapDir := t.TempDir()
	mgr := New(snapDir)

	// Take a snapshot of a file that doesn't yet exist (about to be created).
	target := filepath.Join(repoDir, "new.txt")
	if _, err := mgr.Take("sess1", 0, []string{target}); err != nil {
		t.Fatal(err)
	}

	// Simulate the tool creating the file.
	if err := os.WriteFile(target, []byte("created"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Undo should remove the file (it didn't exist before the step).
	if _, err := mgr.Undo("sess1", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("expected file removed; err=%v", err)
	}
}

func TestSnapshotPrune(t *testing.T) {
	snapDir := t.TempDir()
	mgr := New(snapDir)
	for _, id := range []string{"a", "b", "c"} {
		if err := os.MkdirAll(filepath.Join(snapDir, id, "000000"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	removed, err := mgr.Prune([]string{"a", "c"})
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("removed=%d, want 1", removed)
	}
	if _, err := os.Stat(filepath.Join(snapDir, "b")); !os.IsNotExist(err) {
		t.Fatalf("expected b removed; err=%v", err)
	}
}

// TestSnapshotsSurviveCompactionBoundary pins the invariant that
// snapshot dirs and SQL-layer message indices are independent. The
// snapshot manager keys its dirs by per-session monotonic step
// counter, NOT by message idx, so ReplaceWithCompaction's idx
// rewrites cannot make /undo dereference a stale or missing dir.
//
// The test takes snapshots at three step idx (2, 4, 6), then does
// nothing on the snapshot side to simulate "compaction happened in
// SQL" — and asserts /undo still pops the most-recent step in
// reverse chronological order.
func TestSnapshotsSurviveCompactionBoundary(t *testing.T) {
	repoDir := t.TempDir()
	snapDir := t.TempDir()
	mgr := New(snapDir)

	target := filepath.Join(repoDir, "file.txt")
	if err := os.WriteFile(target, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, step := range []int{2, 4, 6} {
		if _, err := mgr.Take("sess1", step, []string{target}); err != nil {
			t.Fatalf("Take step=%d: %v", step, err)
		}
	}
	if err := os.WriteFile(target, []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}

	// /undo restores the LAST snapshot taken (step 6 dir) regardless
	// of any SQL-layer idx renumbering.
	restored, err := mgr.Undo("sess1", 1)
	if err != nil {
		t.Fatalf("undo: %v", err)
	}
	if restored != 1 {
		t.Errorf("restored = %d, want 1", restored)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "v1" {
		t.Errorf("file content after undo: got %q want %q", got, "v1")
	}

	// The other two step dirs are still intact for follow-up undos.
	for _, name := range []string{"000002", "000004"} {
		if _, err := os.Stat(filepath.Join(snapDir, "sess1", name)); err != nil {
			t.Errorf("step dir %s should still exist after one /undo: %v", name, err)
		}
	}
}
