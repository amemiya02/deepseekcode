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

// isTestPath reports whether f is a test artifact (test source or
// testdata fixture) — the only files scoring may restore from the fix
// commit. Everything else is the solution and must never be touched.
func isTestPath(f string) bool {
	return strings.HasSuffix(f, "_test.go") ||
		strings.HasPrefix(f, "testdata/") ||
		strings.Contains(f, "/testdata/")
}

func splitLines(out []byte) []string {
	s := strings.TrimSpace(string(out))
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// RestoreCanonicalTests resets every test file to its canonical state:
// agent-modified test files are reverted to the buggy commit and
// agent-added ones (tracked or untracked) are removed, then the test
// files the fixing PR changed are checked out from task.FixCommit,
// one file at a time. Whole-directory checkouts from the fix commit
// are forbidden here: for every current task the gold fix's source
// file lives inside test_dir, so a directory checkout would apply the
// solution itself and rig the score.
func (w *Workspace) RestoreCanonicalTests(task TaskSpec) error {
	// 1. Neutralize agent test tampering: revert tracked changes...
	out, _ := exec.Command("git", "-C", w.Dir, "diff", "--name-only", task.Commit).Output()
	for _, f := range splitLines(out) {
		if !isTestPath(f) {
			continue
		}
		if err := exec.Command("git", "-C", w.Dir, "checkout", task.Commit, "--", f).Run(); err != nil {
			os.Remove(filepath.Join(w.Dir, f)) // not in pinned commit: agent-added
		}
	}
	// ...and delete untracked test files, which `git diff` cannot see.
	out, _ = exec.Command("git", "-C", w.Dir, "ls-files", "--others", "--exclude-standard").Output()
	for _, f := range splitLines(out) {
		if isTestPath(f) {
			os.Remove(filepath.Join(w.Dir, f))
		}
	}
	if task.FixCommit == "" {
		return nil
	}
	// 2. Bring in exactly the test files the fixing PR changed (the
	//    fail-to-pass tests were introduced there, SWE-bench style).
	out, err := exec.Command("git", "-C", w.Dir, "diff", "--name-only", task.Commit, task.FixCommit).Output()
	if err != nil {
		return fmt.Errorf("diff %s..%s: %w", task.Commit, task.FixCommit, err)
	}
	for _, f := range splitLines(out) {
		if !isTestPath(f) {
			continue
		}
		if err := exec.Command("git", "-C", w.Dir, "checkout", task.FixCommit, "--", f).Run(); err != nil {
			os.Remove(filepath.Join(w.Dir, f)) // deleted by the fix commit
		}
	}
	return nil
}

// ApplyGoldFix applies the fix commit's NON-test changes — the gold
// solution. Only goldcheck's positive control may call this; applying
// it in a scoring path would invalidate the benchmark.
func (w *Workspace) ApplyGoldFix(task TaskSpec) error {
	out, err := exec.Command("git", "-C", w.Dir, "diff", "--name-only", task.Commit, task.FixCommit).Output()
	if err != nil {
		return fmt.Errorf("diff %s..%s: %w", task.Commit, task.FixCommit, err)
	}
	for _, f := range splitLines(out) {
		if isTestPath(f) {
			continue
		}
		if err := exec.Command("git", "-C", w.Dir, "checkout", task.FixCommit, "--", f).Run(); err != nil {
			// Checkout fails for files the fix commit deleted.
			if rmErr := os.Remove(filepath.Join(w.Dir, f)); rmErr != nil {
				return fmt.Errorf("apply gold fix %s: %w", f, err)
			}
		}
	}
	return nil
}

// RunFailToPass runs the task's fail-to-pass tests with patterns
// anchored ^...$ (so TestFoo cannot match TestFooBar) and a hard
// timeout, returning the combined output.
func (w *Workspace) RunFailToPass(task TaskSpec) (string, error) {
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
	cmd := exec.Command("go", "test", "-count=1", "-timeout", "10m", "-run", strings.Join(anchored, "|"), "-v", task.TestDir)
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
	select {
	case result := <-done:
		return string(result.out), result.err
	case <-time.After(11 * time.Minute):
		cmd.Process.Kill()
		return "", fmt.Errorf("test run exceeded 11m hard kill")
	}
}

// Score restores the canonical fail-to-pass tests (test files only —
// the agent's non-test changes stay as-is), then runs them. Resolved
// means the F2P tests matched, ran, and all passed.
func (w *Workspace) Score(task TaskSpec) bool {
	if err := w.RestoreCanonicalTests(task); err != nil {
		log.Printf("score: restore canonical tests: %v", err)
		return false
	}
	output, err := w.RunFailToPass(task)
	// Hard-fail: "no tests to run" means the pattern matched nothing —
	// exit 0 here must never count as resolved.
	if strings.Contains(output, "no tests to run") || strings.Contains(output, "[no test files]") {
		log.Printf("score: FAIL - no tests matched pattern %q in %s", task.FailToPass, task.Repo)
		return false
	}
	return err == nil && strings.Contains(output, "PASS")
}
