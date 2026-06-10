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
	out, err := exec.Command("git", "-C", w.Dir, "diff", "--name-only", task.Commit).Output()
	if err != nil {
		return fmt.Errorf("diff vs %s: %w", task.Commit, err)
	}
	for _, f := range splitLines(out) {
		if !isTestPath(f) {
			continue
		}
		if err := exec.Command("git", "-C", w.Dir, "checkout", task.Commit, "--", f).Run(); err != nil {
			os.Remove(filepath.Join(w.Dir, f)) // not in pinned commit: agent-added
		}
	}
	// ...and delete untracked test files, which `git diff` cannot see.
	// No --exclude-standard: an agent could gitignore a planted
	// cheat_test.go (e.g. a TestMain that prints PASS) to hide it.
	out, err = exec.Command("git", "-C", w.Dir, "ls-files", "--others").Output()
	if err != nil {
		return fmt.Errorf("ls-files: %w", err)
	}
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
	out, err = exec.Command("git", "-C", w.Dir, "diff", "--name-only", task.Commit, task.FixCommit).Output()
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
			// Only treat a checkout failure as "deleted by the fix
			// commit" when the file truly isn't there — anything else
			// (corrupt object, transient git failure) must not silently
			// delete a needed source file.
			if exec.Command("git", "-C", w.Dir, "cat-file", "-e", task.FixCommit+":"+f).Run() == nil {
				return fmt.Errorf("apply gold fix: checkout %s from %s: %w", f, task.FixCommit, err)
			}
			if rmErr := os.Remove(filepath.Join(w.Dir, f)); rmErr != nil {
				return fmt.Errorf("apply gold fix %s: %w", f, err)
			}
		}
	}
	return nil
}

// anchorPattern anchors EVERY slash segment of a -run pattern with
// ^...$. go test splits -run on "/" and matches each level separately,
// so "^Test/Foo$" would become ^Test + Foo$ and run every wrapper
// suite in the package; "^Test$/^Foo$" runs only the targeted subtest
// (and TestFoo cannot match TestFooBar).
func anchorPattern(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		if !strings.HasPrefix(s, "^") {
			s = "^" + s
		}
		if !strings.HasSuffix(s, "$") {
			s = s + "$"
		}
		segs[i] = s
	}
	return strings.Join(segs, "/")
}

// RunFailToPass runs the task's fail-to-pass tests with per-segment
// anchored patterns and a hard timeout, returning the combined output.
func (w *Workspace) RunFailToPass(task TaskSpec) (string, error) {
	var anchored []string
	for _, p := range task.FailToPass {
		anchored = append(anchored, anchorPattern(p))
	}
	cmd := exec.Command("go", "test", "-count=1", "-timeout", "10m", "-run", strings.Join(anchored, "|"), "-v", task.TestDir)
	cmd.Dir = w.Dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	// Own process group so the hard kill reaps compiled test binaries,
	// not just the go tool.
	cmd.SysProcAttr = setSysProcAttr(nil)
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
		if cmd.Process != nil {
			killProcessGroup(cmd.Process.Pid)
		}
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
