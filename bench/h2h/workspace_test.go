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

// makeFixtureRepoWithFix builds a repo SWE-bench style: the buggy
// commit has no test; the fix commit fixes the source AND adds the
// fail-to-pass test (like an upstream fixing PR).
func makeFixtureRepoWithFix(t *testing.T) (dir, buggy, fix string) {
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
	head := func() string {
		out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(out))
	}
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module fixture\n\ngo 1.21\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "add.go"), []byte("package fixture\n\nfunc Add(a, b int) int { return a - b } // bug\n"), 0o644)
	run("git", "init", "-q")
	run("git", "add", "-A")
	run("git", "commit", "-qm", "buggy")
	buggy = head()
	os.WriteFile(filepath.Join(dir, "add.go"), []byte("package fixture\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "add_test.go"), []byte("package fixture\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(2, 2) != 4 {\n\t\tt.Fatal(\"add broken\")\n\t}\n}\n"), 0o644)
	run("git", "add", "-A")
	run("git", "commit", "-qm", "fix + add F2P test")
	fix = head()
	run("git", "checkout", "-q", buggy)
	return dir, buggy, fix
}

// TestScoreDoesNotLeakGoldFix is the regression test for the critical
// scoring bug: Score must take ONLY test files from the fix commit,
// never the solution itself.
func TestScoreDoesNotLeakGoldFix(t *testing.T) {
	src, buggy, fix := makeFixtureRepoWithFix(t)
	task := TaskSpec{ID: "fix-add", Repo: src, Commit: buggy, FixCommit: fix, Prompt: "fix Add",
		FailToPass: []string{"TestAdd"}, TestDir: "./...", TurnCap: 5, WallclockCapMin: 5}

	ws, err := NewWorkspace(t.TempDir(), task)
	if err != nil {
		t.Fatalf("NewWorkspace: %v", err)
	}
	// An untouched workspace must NOT be resolved — if it is, scoring
	// applied the gold fix.
	if ws.Score(task) {
		t.Fatal("unfixed workspace scored resolved: gold fix leaked into scoring")
	}
	// The buggy source must be untouched by scoring...
	b, err := os.ReadFile(filepath.Join(ws.Dir, "add.go"))
	if err != nil || !strings.Contains(string(b), "a - b") {
		t.Fatalf("Score modified non-test file add.go: %s (%v)", b, err)
	}
	// ...while the canonical test was brought in from the fix commit.
	if _, err := os.Stat(filepath.Join(ws.Dir, "add_test.go")); err != nil {
		t.Fatal("canonical F2P test not restored from fix commit")
	}
	// Agent fixes the bug, tampers with the canonical test, and plants
	// an extra always-pass test: only the real fix may count.
	os.WriteFile(filepath.Join(ws.Dir, "add.go"), []byte("package fixture\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644)
	os.WriteFile(filepath.Join(ws.Dir, "add_test.go"), []byte("package fixture\n"), 0o644)
	os.WriteFile(filepath.Join(ws.Dir, "extra_test.go"), []byte("package fixture\n\nimport \"testing\"\n\nfunc TestAdd2(t *testing.T) {}\n"), 0o644)
	if !ws.Score(task) {
		t.Fatal("fixed workspace must score resolved (canonical test restored, fix preserved)")
	}
	if _, err := os.Stat(filepath.Join(ws.Dir, "extra_test.go")); !os.IsNotExist(err) {
		t.Fatal("agent-added untracked test file survived scoring")
	}
}

// TestApplyGoldFixThenScore is goldcheck's positive control at the
// library level: the gold solution alone must make Score() resolve.
func TestApplyGoldFixThenScore(t *testing.T) {
	src, buggy, fix := makeFixtureRepoWithFix(t)
	task := TaskSpec{ID: "fix-add", Repo: src, Commit: buggy, FixCommit: fix, Prompt: "fix Add",
		FailToPass: []string{"TestAdd"}, TestDir: "./...", TurnCap: 5, WallclockCapMin: 5}

	ws, err := NewWorkspace(t.TempDir(), task)
	if err != nil {
		t.Fatalf("NewWorkspace: %v", err)
	}
	// Negative control: canonical tests must run and FAIL at buggy.
	if err := ws.RestoreCanonicalTests(task); err != nil {
		t.Fatalf("RestoreCanonicalTests: %v", err)
	}
	output, runErr := ws.RunFailToPass(task)
	if runErr == nil || !strings.Contains(output, "--- FAIL: TestAdd") {
		t.Fatalf("F2P test did not run-and-fail at buggy commit: err=%v\n%s", runErr, output)
	}
	// Positive control: apply gold fix (non-test files only) → resolved.
	if err := ws.ApplyGoldFix(task); err != nil {
		t.Fatalf("ApplyGoldFix: %v", err)
	}
	if !ws.Score(task) {
		t.Fatal("Score() must resolve after applying the gold fix")
	}
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
