package h2h

import (
	"fmt"
	"os/exec"
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

// Score restores the canonical fail-to-pass tests from the pinned
// commit (the agent may have edited them — restore ONLY *_test.go so
// the agent's fix survives), then runs them. Resolved means every F2P
// test passes within 10 minutes.
func (w *Workspace) Score(task TaskSpec) bool {
	out, _ := exec.Command("git", "-C", w.Dir, "diff", "--name-only", task.Commit).Output()
	for _, f := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.HasSuffix(f, "_test.go") {
			exec.Command("git", "-C", w.Dir, "checkout", task.Commit, "--", f).Run()
		}
	}
	run := strings.Join(task.FailToPass, "|")
	cmd := exec.Command("go", "test", task.TestDir, "-run", run, "-count=1", "-timeout", "10m")
	cmd.Dir = w.Dir
	if err := cmd.Start(); err != nil {
		return false
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err == nil
	case <-time.After(11 * time.Minute):
		cmd.Process.Kill()
		return false
	}
}
