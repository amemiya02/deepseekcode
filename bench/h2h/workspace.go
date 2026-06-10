package h2h

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Workspace is a fresh checkout of a task's repo at its pinned commit.
type Workspace struct {
	Dir string
}

// NewWorkspace clones task.Repo into parent/<task.ID> and checks out
// task.Commit. A fresh clone per task per run is fairness rule §3.3(3).
func NewWorkspace(parent string, task TaskSpec) (*Workspace, error) {
	dir := parent + "/" + task.ID
	if out, err := exec.Command("git", "clone", "-q", task.Repo, dir).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("clone: %s: %w", out, err)
	}
	if out, err := exec.Command("git", "-C", dir, "checkout", "-q", task.Commit).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("checkout %s: %s: %w", task.Commit, out, err)
	}
	return &Workspace{Dir: dir}, nil
}

// Score restores the canonical fail-to-pass tests from the fix commit
// (SWE-bench tasks have tests added by the fixing PR, not present at
// the buggy commit), then runs them. Resolved means every F2P test
// passes within 10 minutes and at least one test actually ran.
func (w *Workspace) Score(task TaskSpec) bool {
	// 1. Checkout test files from the fix commit, where the failing
	//    tests were actually introduced. The agent's code changes
	//    (non-test files) stay as-is from whatever commit it produced.
	if task.FixCommit != "" {
		testDir := task.TestDir
		if testDir == "" {
			testDir = "."
		}
		// Strip trailing "/..." for git checkout pathspec
		gitPath := strings.TrimSuffix(testDir, "/...")
		if gitPath == "" {
			gitPath = "."
		}
		cmd := exec.Command("git", "checkout", task.FixCommit, "--", gitPath)
		cmd.Dir = w.Dir
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("score: checkout fix tests from %s: %v\n%s", task.FixCommit, err, out)
			return false
		}
	} else {
		// Fallback: restore test files from the buggy commit
		// (legacy behavior for tasks without fix_commit).
		out, _ := exec.Command("git", "-C", w.Dir, "diff", "--name-only", task.Commit).Output()
		for _, f := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if !strings.HasSuffix(f, "_test.go") {
				continue
			}
			// Restore canonical test from pinned commit. If the file
			// did not exist at the pinned commit (agent-added test),
			// remove it so it cannot interfere with scoring.
			if err := exec.Command("git", "-C", w.Dir, "checkout", task.Commit, "--", f).Run(); err != nil {
				os.Remove(filepath.Join(w.Dir, f))
			}
		}
	}

	// 2. Run the tests with timeout and capture output.
	// Anchor patterns to avoid false matches (e.g. TestFoo matching TestFooBar).
	var anchored []string
	for _, p := range task.FailToPass {
		if !strings.HasPrefix(p, "^") {
			p = "^" + p
		}
		if !strings.HasSuffix(p, "$") {
			p = p + "$"
		}
		anchored = append(anchored, p)
	}
	run := strings.Join(anchored, "|")
	cmd := exec.Command("go", "test", "-count=1", "-timeout", "10m", "-run", run, "-v", task.TestDir)
	cmd.Dir = w.Dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	done := make(chan struct {
		out []byte
		err error
	}, 1)
	go func() {
		out, err := cmd.CombinedOutput()
		done <- struct {
			out []byte
			err error
		}{out, err}
	}()

	var output string
	var err error
	select {
	case result := <-done:
		output = string(result.out)
		err = result.err
	case <-time.After(11 * time.Minute):
		cmd.Process.Kill()
		return false
	}

	// 3. Hard-fail: "no tests to run" means pattern didn't match.
	if strings.Contains(output, "no tests to run") || strings.Contains(output, "[no test files]") {
		log.Printf("score: FAIL - no tests matched pattern %q in %s", task.FailToPass, task.Repo)
		return false
	}

	// 4. Hard-fail: zero tests ran.
	if strings.Contains(output, "Tests: 0 ") || strings.Contains(output, "0 passed") {
		log.Printf("score: FAIL - 0 tests ran for pattern %q", task.FailToPass)
		return false
	}

	return err == nil && strings.Contains(output, "PASS")
}
