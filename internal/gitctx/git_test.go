package gitctx

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.name=t", "-c", "user.email=t@e", "commit", "--allow-empty", "-q", "-m", "first"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestReadFreshRepo(t *testing.T) {
	dir := initRepo(t)
	r := &Reader{CWD: dir}
	snap, err := r.Read(context.Background())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if snap.ActiveBranch == "" {
		t.Errorf("ActiveBranch empty on a fresh repo")
	}
	if len(snap.RecentCommits) == 0 {
		t.Errorf("RecentCommits empty after seeding one commit")
	}
}

func TestReadWithChanges(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "a.go", "package a\n")
	r := &Reader{CWD: dir}
	snap, err := r.Read(context.Background())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(snap.Status, "a.go") {
		t.Errorf("status missing untracked a.go: %q", snap.Status)
	}
}

func TestReadNotARepo(t *testing.T) {
	dir := t.TempDir()
	r := &Reader{CWD: dir}
	snap, err := r.Read(context.Background())
	if err != nil {
		t.Fatalf("non-repo cwd should not error: %v", err)
	}
	if snap.ActiveBranch != "" || snap.Status != "" {
		t.Errorf("non-repo snapshot should be empty: %+v", snap)
	}
}

func TestReadMissingCwd(t *testing.T) {
	r := &Reader{CWD: filepath.Join(t.TempDir(), "does-not-exist")}
	if _, err := r.Read(context.Background()); err == nil {
		t.Error("expected error for missing cwd")
	}
}

func TestReaderCacheTTL(t *testing.T) {
	dir := initRepo(t)
	r := &Reader{CWD: dir, CacheTTL: 5 * time.Second}
	for i := 0; i < 3; i++ {
		if _, err := r.Read(context.Background()); err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
	}
	if got := r.runCount(); got != 1 {
		t.Errorf("git invocations: got %d, want 1 (cache hits expected for 2nd/3rd Read)", got)
	}
}

func TestReaderCacheExpires(t *testing.T) {
	dir := initRepo(t)
	r := &Reader{CWD: dir, CacheTTL: 10 * time.Millisecond}
	if _, err := r.Read(context.Background()); err != nil {
		t.Fatalf("read 1: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if _, err := r.Read(context.Background()); err != nil {
		t.Fatalf("read 2: %v", err)
	}
	if got := r.runCount(); got != 2 {
		t.Errorf("git invocations: got %d, want 2 (TTL expired between reads)", got)
	}
}

func TestReadEmptyCwd(t *testing.T) {
	r := &Reader{}
	if _, err := r.Read(context.Background()); err == nil {
		t.Error("expected error for empty CWD")
	}
}
