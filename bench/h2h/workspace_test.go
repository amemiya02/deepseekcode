package h2h

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// makeFixtureRepo builds a tiny git repo with a failing test at HEAD.
func makeFixtureRepo(t *testing.T) (dir, commit string) {
	t.Helper()
	dir = t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", args, out)
		}
	}
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module fixture\n\ngo 1.21\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "add.go"), []byte("package fixture\n\nfunc Add(a, b int) int { return a - b } // bug\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "add_test.go"), []byte("package fixture\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(2, 2) != 4 {\n\t\tt.Fatal(\"add broken\")\n\t}\n}\n"), 0o644)
	run("git", "init", "-q")
	run("git", "add", "-A")
	run("git", "commit", "-qm", "fixture")
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	return dir, strings.TrimSpace(string(out))
}

func TestWorkspaceCloneAndScore(t *testing.T) {
	src, commit := makeFixtureRepo(t)
	task := TaskSpec{ID: "fix-add", Repo: src, Commit: commit, Prompt: "fix Add",
		FailToPass: []string{"TestAdd"}, TestDir: "./...", TurnCap: 5, WallclockCapMin: 5}

	ws, err := NewWorkspace(t.TempDir(), task)
	if err != nil {
		t.Fatalf("NewWorkspace: %v", err)
	}
	// Unfixed: scoring must report not-resolved.
	if ws.Score(task) {
		t.Fatal("unfixed fixture must not score as resolved")
	}
	// Simulate the agent fixing the bug AND tampering with the test.
	os.WriteFile(filepath.Join(ws.Dir, "add.go"), []byte("package fixture\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644)
	os.WriteFile(filepath.Join(ws.Dir, "add_test.go"), []byte("package fixture\n"), 0o644) // tamper
	// Score must restore the canonical test before running it — but
	// must NOT revert the agent's fix to add.go.
	if !ws.Score(task) {
		t.Fatal("fixed fixture must score as resolved (canonical test restored, fix preserved)")
	}
}
