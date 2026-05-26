package worktree

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// skipIfNoGit skips the test if git is not available.
func skipIfNoGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

// initGitRepo creates a temporary git repository, returns its path
// and a cleanup function.
func initGitRepo(t *testing.T) (string, func()) {
	t.Helper()
	skipIfNoGit(t)

	dir, err := os.MkdirTemp("", "worktree-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}

	cleanup := func() { os.RemoveAll(dir) }

	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	// Create an initial commit so HEAD exists.
	writeFile(t, filepath.Join(dir, "README.md"), "# test repo")
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "initial")

	return dir, cleanup
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %s: %v", strings.Join(args, " "), string(out), err)
	}
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeFile: %v", err)
	}
}

func TestNewManager(t *testing.T) {
	m := NewManager("/tmp/repo")
	if m.Root != "/tmp/repo" {
		t.Errorf("Root = %q, want /tmp/repo", m.Root)
	}
}

func TestValidateBranch(t *testing.T) {
	tests := []struct {
		name string
		ok   bool
	}{
		{"", false},
		{"  ", false},
		{"HEAD", false},
		{"/evil", false},
		{"../escape", false},
		{"a;b", false},
		{"a b", false},
		{"a'b", false},
		{"a|b", false},
		{"a&b", false},
		{"a$b", false},
		{"a(b", false},
		{"feat/x", true},
		{"subagent/explore-1700000000", true},
		{"fix-42", true},
		{"a.b-c_d", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateBranch(tc.name)
			if tc.ok && err != nil {
				t.Errorf("expected ok, got %v", err)
			}
			if !tc.ok && err == nil {
				t.Errorf("expected error for %q", tc.name)
			}
		})
	}
}

func TestManagerCreateListRemove(t *testing.T) {
	dir, cleanup := initGitRepo(t)
	defer cleanup()

	ctx := context.Background()
	m := NewManager(dir)

	// Create a worktree.
	wt, err := m.Create(ctx, "feat/x", "HEAD")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if wt.Branch != "feat/x" {
		t.Errorf("Branch = %q, want feat/x", wt.Branch)
	}
	if !strings.HasPrefix(wt.Path, filepath.Join(dir, ".deepseek", "worktrees")) {
		t.Errorf("Path = %q, want under .deepseek/worktrees/", wt.Path)
	}
	if wt.Head == "" {
		t.Error("Head should not be empty")
	}

	// Verify the path exists.
	if _, err := os.Stat(wt.Path); os.IsNotExist(err) {
		t.Errorf("worktree path does not exist: %s", wt.Path)
	}

	// Verify branch in new worktree.
	branchOut := runGit(t, wt.Path, "branch", "--show-current")
	if branchOut != "feat/x" {
		t.Errorf("branch --show-current = %q, want feat/x", branchOut)
	}

	// List should include main worktree + feat/x.
	list, err := m.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) < 2 {
		t.Fatalf("List returned %d worktrees, want >= 2", len(list))
	}
	found := false
	for _, w := range list {
		if w.Branch == "feat/x" {
			found = true
			break
		}
	}
	if !found {
		t.Error("List should contain feat/x")
	}

	// Remove the worktree (should be clean).
	if err := m.Remove(ctx, "feat/x", false); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Path should be gone.
	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Errorf("worktree path should be removed: %s", wt.Path)
	}
}

func TestManagerCreateDirtyRemove(t *testing.T) {
	dir, cleanup := initGitRepo(t)
	defer cleanup()

	ctx := context.Background()
	m := NewManager(dir)

	wt, err := m.Create(ctx, "feat/y", "HEAD")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Make the worktree dirty by modifying a file.
	writeFile(t, filepath.Join(wt.Path, "dirty.txt"), "changed")
	runGit(t, wt.Path, "add", "dirty.txt")

	// Remove without force should fail.
	if err := m.Remove(ctx, "feat/y", false); err == nil {
		t.Fatal("expected error removing dirty worktree without --force")
	}

	// Remove with force should succeed.
	if err := m.Remove(ctx, "feat/y", true); err != nil {
		t.Fatalf("Remove --force: %v", err)
	}
}

func TestManagerCreateAlreadyExists(t *testing.T) {
	dir, cleanup := initGitRepo(t)
	defer cleanup()

	ctx := context.Background()
	m := NewManager(dir)

	_, err := m.Create(ctx, "feat/z", "HEAD")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Second create with same branch should fail.
	_, err = m.Create(ctx, "feat/z", "HEAD")
	if err == nil {
		t.Fatal("expected error for duplicate branch")
	}
	if !errors.Is(err, ErrAlreadyExists) {
		t.Errorf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestManagerCreateInvalidBranch(t *testing.T) {
	dir, cleanup := initGitRepo(t)
	defer cleanup()

	ctx := context.Background()
	m := NewManager(dir)

	_, err := m.Create(ctx, "; rm -rf /", "HEAD")
	if err == nil {
		t.Fatal("expected error for invalid branch name")
	}
	if !errors.Is(err, ErrInvalidBranch) {
		t.Errorf("expected ErrInvalidBranch, got %v", err)
	}
}

func TestManagerPrune(t *testing.T) {
	dir, cleanup := initGitRepo(t)
	defer cleanup()

	ctx := context.Background()
	m := NewManager(dir)

	if err := m.Prune(ctx); err != nil {
		t.Fatalf("Prune: %v", err)
	}
}

func TestManagerStatus(t *testing.T) {
	dir, cleanup := initGitRepo(t)
	defer cleanup()

	ctx := context.Background()
	m := NewManager(dir)

	_, err := m.Create(ctx, "feat/s", "HEAD")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	wt, err := m.Status(ctx, "feat/s")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if wt.Branch != "feat/s" {
		t.Errorf("Branch = %q, want feat/s", wt.Branch)
	}
	if wt.Head == "" {
		t.Error("Head should not be empty")
	}

	// Unknown branch.
	_, err = m.Status(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for unknown branch")
	}
}

func TestParsePorcelain(t *testing.T) {
	output := `worktree /repo
HEAD abc123
branch refs/heads/main

worktree /repo/.deepseek/worktrees/feat/x
HEAD def456
branch refs/heads/feat/x
locked

`
	list := parsePorcelain(output)
	if len(list) != 2 {
		t.Fatalf("expected 2 worktrees, got %d", len(list))
	}

	if list[0].Path != "/repo" || list[0].Branch != "main" || list[0].Head != "abc123" || list[0].Locked {
		t.Errorf("first worktree: %+v", list[0])
	}
	if list[1].Path != "/repo/.deepseek/worktrees/feat/x" || list[1].Branch != "feat/x" || list[1].Head != "def456" || !list[1].Locked {
		t.Errorf("second worktree: %+v", list[1])
	}
}
