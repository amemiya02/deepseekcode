// Package gitctx reads a cwd's git state for the prompt builder's
// dynamic context section.
//
// Design choices:
//   - Subprocess only (no go-git). Reuses the user's installed git
//     binary, so behavior matches what the user sees on the CLI.
//   - Read-only. Never invokes `git add`, `commit`, or any mutating
//     operation.
//   - Failure-tolerant. A missing git binary, a non-repo cwd, or any
//     individual command failure returns a partial Snapshot with a
//     nil error — git context is advisory, not load-bearing.
package gitctx

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	defaultTimeout    = 2 * time.Second
	maxStatusChars    = 4000
	maxDiffCharsTotal = 8000
)

// Reader reads git state from a cwd. Construct directly; the zero
// value with a populated CWD is usable (defaultTimeout applies).
type Reader struct {
	CWD     string
	Timeout time.Duration
}

// Snapshot is one read of the cwd's git state. All fields are
// optional — a non-git cwd yields a zero-value Snapshot with nil
// error.
type Snapshot struct {
	Status        string   // git status --short, truncated
	Diff          string   // staged + unstaged --stat, combined and truncated
	ActiveBranch  string   // git rev-parse --abbrev-ref HEAD
	RecentCommits []string // git log -3 --oneline
}

// Read collects the snapshot. Each git command is bounded by the
// shorter of ctx and r.Timeout (defaults to 2s).
//
// Errors are returned only when the cwd itself is malformed (e.g.
// the directory doesn't exist). Individual git command failures are
// swallowed — the corresponding Snapshot fields stay empty.
func (r *Reader) Read(ctx context.Context) (Snapshot, error) {
	if r.CWD == "" {
		return Snapshot{}, errors.New("gitctx: empty CWD")
	}
	if _, err := os.Stat(r.CWD); err != nil {
		return Snapshot{}, err
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	var out Snapshot
	out.Status = truncate(runGit(ctx, r.CWD, timeout, "status", "--short"), maxStatusChars)
	unstaged := runGit(ctx, r.CWD, timeout, "diff", "--stat")
	staged := runGit(ctx, r.CWD, timeout, "diff", "--cached", "--stat")
	out.Diff = truncate(joinNonEmpty(unstaged, staged), maxDiffCharsTotal)
	out.ActiveBranch = strings.TrimSpace(runGit(ctx, r.CWD, timeout, "rev-parse", "--abbrev-ref", "HEAD"))
	if log := runGit(ctx, r.CWD, timeout, "log", "-3", "--oneline", "--no-color"); log != "" {
		for _, line := range strings.Split(strings.TrimRight(log, "\n"), "\n") {
			if line == "" {
				continue
			}
			out.RecentCommits = append(out.RecentCommits, line)
		}
	}
	return out, nil
}

func runGit(ctx context.Context, cwd string, timeout time.Duration, args ...string) string {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "git", append([]string{"-C", cwd}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_PAGER=", "GIT_TERMINAL_PROMPT=0")
	b, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(b)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n... [truncated]"
}

func joinNonEmpty(parts ...string) string {
	var b strings.Builder
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(p)
	}
	return b.String()
}
