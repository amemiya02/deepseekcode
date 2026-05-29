package snapshots

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// TestSnapshotTakeIsDurable asserts Take writes the snapshot via an
// atomic rename rather than streaming directly into the final path: while
// a copy is in flight no truncated payload exists at the snapshot's final
// name. We verify the steady-state result here — the real snapshot exists,
// matches the source byte-for-byte, and no leftover temp sibling remains.
func TestSnapshotTakeIsDurable(t *testing.T) {
	repoDir := t.TempDir()
	snapDir := t.TempDir()
	mgr := New(snapDir)

	target := filepath.Join(repoDir, "big.txt")
	want := strings.Repeat("payload-", 4096) // ~32 KiB, multiple write chunks
	if err := os.WriteFile(target, []byte(want), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Take("sess1", 0, []string{target}); err != nil {
		t.Fatal(err)
	}

	stepDir := filepath.Join(snapDir, "sess1", "000000")
	entries, err := os.ReadDir(stepDir)
	if err != nil {
		t.Fatal(err)
	}
	var snapName string
	for _, e := range entries {
		name := e.Name()
		if strings.Contains(name, ".tmp-") {
			t.Fatalf("leftover temp file after Take: %q", name)
		}
		snapName = name
	}
	if snapName == "" {
		t.Fatal("no snapshot file written")
	}
	got, err := os.ReadFile(filepath.Join(stepDir, snapName))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("snapshot payload mismatch: got %d bytes, want %d", len(got), len(want))
	}
}

// TestSnapshotCrashMidWriteDoesNotCorruptRestore simulates a crash that
// happened *during* a snapshot write: a stray temp file is left behind in
// the step dir, but the previously-completed snapshot is intact. The
// invariant: Undo ignores the truncated temp file and restores the good
// snapshot, so an interrupted write never restores a partial file as if
// complete.
func TestSnapshotCrashMidWriteDoesNotCorruptRestore(t *testing.T) {
	repoDir := t.TempDir()
	snapDir := t.TempDir()
	mgr := New(snapDir)

	target := filepath.Join(repoDir, "code.go")
	if err := os.WriteFile(target, []byte("GOOD-ORIGINAL"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Take("sess1", 0, []string{target}); err != nil {
		t.Fatal(err)
	}

	// Simulate a crash mid-write: a half-flushed temp sibling for the same
	// target was left in the step dir before fsync/rename completed. Its
	// content is garbage/truncated.
	stepDir := filepath.Join(snapDir, "sess1", "000000")
	snapName := encodePath(mustAbs(t, target))
	tmpJunk := filepath.Join(stepDir, "."+snapName+".tmp-deadbeef")
	if err := os.WriteFile(tmpJunk, []byte("TRUNCATED-GARB"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The working tree was mutated after the snapshot.
	if err := os.WriteFile(target, []byte("MUTATED"), 0o644); err != nil {
		t.Fatal(err)
	}

	restored, err := mgr.Undo("sess1", 1)
	if err != nil {
		t.Fatal(err)
	}
	// Only the real snapshot restores; the temp file is not a valid
	// encoded path and must be skipped.
	if restored != 1 {
		t.Fatalf("Undo restored %d, want 1 (temp file must be ignored)", restored)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "GOOD-ORIGINAL" {
		t.Fatalf("restore corrupted: got %q, want %q", got, "GOOD-ORIGINAL")
	}
}

// TestSnapshotPruneTrimsToKeptSet exercises Prune beyond the single-removal
// happy path: it must remove every session dir absent from the keep set
// (including nested step dirs), keep all kept ones intact, and report an
// accurate removed count. A keep set referencing a non-existent session is
// a no-op for that entry.
func TestSnapshotPruneTrimsToKeptSet(t *testing.T) {
	snapDir := t.TempDir()
	mgr := New(snapDir)

	sessions := []string{"keep1", "drop1", "keep2", "drop2", "drop3"}
	for _, id := range sessions {
		// Each session gets a couple of step dirs with a file inside, so
		// we prove Prune trims nested state, not just empty top dirs.
		for _, step := range []string{"000000", "000001"} {
			d := filepath.Join(snapDir, id, step)
			if err := os.MkdirAll(d, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(d, "x"), []byte(id), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	// A stray non-directory entry at the root must be left untouched.
	if err := os.WriteFile(filepath.Join(snapDir, "README"), []byte("meta"), 0o644); err != nil {
		t.Fatal(err)
	}

	// "ghost" is in the keep set but never existed on disk — harmless.
	removed, err := mgr.Prune([]string{"keep1", "keep2", "ghost"})
	if err != nil {
		t.Fatal(err)
	}
	if removed != 3 {
		t.Fatalf("removed=%d, want 3", removed)
	}

	for _, id := range []string{"keep1", "keep2"} {
		if _, err := os.Stat(filepath.Join(snapDir, id, "000000", "x")); err != nil {
			t.Errorf("kept session %s should be intact: %v", id, err)
		}
	}
	for _, id := range []string{"drop1", "drop2", "drop3"} {
		if _, err := os.Stat(filepath.Join(snapDir, id)); !os.IsNotExist(err) {
			t.Errorf("dropped session %s should be gone; err=%v", id, err)
		}
	}
	if _, err := os.Stat(filepath.Join(snapDir, "README")); err != nil {
		t.Errorf("non-dir root entry must be preserved: %v", err)
	}

	// An empty keep set trims everything that remains.
	removed, err = mgr.Prune(nil)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 2 {
		t.Fatalf("second prune removed=%d, want 2", removed)
	}
}

// TestSnapshotConcurrentTakeUndoRaceClean drives Take/Undo/Prune/HasSnapshots
// from many goroutines at once. Its purpose is to be run under `go test
// -race`: the Manager mutex must serialize all directory-tree mutation so
// the detector finds no data race. Each session is independent so the
// outcome stays deterministic enough to keep the harness honest.
func TestSnapshotConcurrentTakeUndoRaceClean(t *testing.T) {
	repoDir := t.TempDir()
	snapDir := t.TempDir()
	mgr := New(snapDir)

	const workers = 16
	// keepAll lets every worker call Prune to stir the shared root listing
	// under the lock without ever deleting another worker's live session,
	// keeping the outcome deterministic while still exercising the mutex.
	keepAll := make([]string, workers)
	for w := 0; w < workers; w++ {
		keepAll[w] = fmt.Sprintf("sess-%d", w)
	}

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func(w int) {
			defer wg.Done()
			sess := fmt.Sprintf("sess-%d", w)
			target := filepath.Join(repoDir, fmt.Sprintf("f-%d.txt", w))
			if err := os.WriteFile(target, []byte("v0"), 0o644); err != nil {
				t.Errorf("seed write: %v", err)
				return
			}
			for step := 0; step < 8; step++ {
				if _, err := mgr.Take(sess, step, []string{target}); err != nil {
					t.Errorf("Take: %v", err)
					return
				}
				_ = os.WriteFile(target, []byte(fmt.Sprintf("v%d", step+1)), 0o644)
				_ = mgr.HasSnapshots(sess)
				if _, err := mgr.Undo(sess, 1); err != nil {
					t.Errorf("Undo: %v", err)
					return
				}
				if _, err := mgr.Prune(keepAll); err != nil {
					t.Errorf("Prune: %v", err)
					return
				}
			}
		}(w)
	}
	wg.Wait()
}

func mustAbs(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatalf("abs %s: %v", p, err)
	}
	return abs
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
