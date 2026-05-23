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
