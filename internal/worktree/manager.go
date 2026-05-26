// Package worktree provides git-worktree management via shell commands.
// It uses exec.Command("git", ...) — no go-git dependency.
package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Worktree represents a single git worktree.
type Worktree struct {
	Branch string // branch name (e.g. "feat/x")
	Path   string // absolute filesystem path
	Head   string // commit SHA at HEAD
	Locked bool   // whether git reports this worktree as locked
	Dirty  bool   // whether git status --porcelain reports uncommitted changes
}

// Manager manages git worktrees under a project root.
type Manager struct {
	Root string // project root; worktrees go to <Root>/.deepseek/worktrees/
}

// Sentinel errors.
var (
	ErrInvalidBranch = errors.New("invalid branch name")
	ErrAlreadyExists = errors.New("worktree directory already exists")
)

// NewManager returns a Manager for the given project root.
func NewManager(projectRoot string) *Manager {
	return &Manager{Root: projectRoot}
}

// worktreesDir returns the path to the worktrees directory.
func (m *Manager) worktreesDir() string {
	return filepath.Join(m.Root, ".deepseek", "worktrees")
}

// validateBranch rejects branch names that could cause command injection
// or filesystem traversal.
func validateBranch(name string) error {
	if name == "" || strings.TrimSpace(name) == "" {
		return ErrInvalidBranch
	}
	if name == "HEAD" {
		return ErrInvalidBranch
	}
	if strings.HasPrefix(name, "/") {
		return ErrInvalidBranch
	}
	if strings.Contains(name, "..") {
		return ErrInvalidBranch
	}
	for _, c := range name {
		switch c {
		case ';', ' ', '\'', '"', '`', '|', '&', '$', '(', ')', '<', '>', '\\':
			return ErrInvalidBranch
		}
	}
	return nil
}

// Create creates a new worktree with the given branch name, based on baseRev.
// Equivalent to: git worktree add -b <branch> <Root>/.deepseek/worktrees/<branch> <baseRev>
func (m *Manager) Create(ctx context.Context, branch, baseRev string) (Worktree, error) {
	if err := validateBranch(branch); err != nil {
		return Worktree{}, err
	}
	if baseRev == "" {
		baseRev = "HEAD"
	}

	path := filepath.Join(m.worktreesDir(), branch)

	// Check if directory already exists.
	if _, err := os.Stat(path); err == nil {
		return Worktree{}, fmt.Errorf("%w: %s", ErrAlreadyExists, path)
	}

	// Ensure .deepseek/worktrees/ parent exists.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Worktree{}, fmt.Errorf("mkdir worktrees parent: %w", err)
	}

	// Ensure .deepseek/.gitignore includes worktrees/.
	gitignorePath := filepath.Join(m.Root, ".deepseek", ".gitignore")
	if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(gitignorePath), 0o755); err != nil {
			return Worktree{}, fmt.Errorf("mkdir .deepseek: %w", err)
		}
		if err := os.WriteFile(gitignorePath, []byte("worktrees/\n"), 0o644); err != nil {
			return Worktree{}, fmt.Errorf("write .gitignore: %w", err)
		}
	} else {
		// Append worktrees/ if not already gitignored.
		content, err := os.ReadFile(gitignorePath)
		if err == nil && !strings.Contains(string(content), "worktrees") {
			f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_WRONLY, 0o644)
			if err == nil {
				if _, werr := f.WriteString("worktrees/\n"); werr != nil {
					f.Close()
					return Worktree{}, fmt.Errorf("write .gitignore append: %w", werr)
				}
				if cerr := f.Close(); cerr != nil {
					return Worktree{}, fmt.Errorf("close .gitignore: %w", cerr)
				}
			}
		}
	}

	// Execute git worktree add.
	args := []string{"worktree", "add", "-b", branch, path}
	if baseRev != "HEAD" {
		args = append(args, baseRev)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = m.Root
	out, err := cmd.CombinedOutput()
	if err != nil {
		return Worktree{}, fmt.Errorf("git worktree add: %s: %w", strings.TrimSpace(string(out)), err)
	}

	// Read HEAD SHA from the new worktree.
	headOut, err := exec.CommandContext(ctx, "git", "-C", path, "rev-parse", "HEAD").Output()
	if err != nil {
		return Worktree{}, fmt.Errorf("git rev-parse HEAD: %w", err)
	}

	return Worktree{
		Branch: branch,
		Path:   path,
		Head:   strings.TrimSpace(string(headOut)),
	}, nil
}

// Remove removes a worktree by branch name. If force is true, uses --force.
func (m *Manager) Remove(ctx context.Context, branch string, force bool) error {
	path := filepath.Join(m.worktreesDir(), branch)

	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, path)

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = m.Root
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree remove: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// List returns all worktrees for this repository, including the main worktree.
// Parses `git worktree list --porcelain`.
func (m *Manager) List(ctx context.Context) ([]Worktree, error) {
	cmd := exec.CommandContext(ctx, "git", "worktree", "list", "--porcelain")
	cmd.Dir = m.Root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}

	return parsePorcelain(string(out)), nil
}

// parsePorcelain parses the output of `git worktree list --porcelain`.
// Records are separated by blank lines; each record has:
//
//	worktree <path>
//	HEAD <sha>
//	branch <ref>   (optional — detached HEAD worktree omits this)
//	locked         (optional)
func parsePorcelain(output string) []Worktree {
	var (
		worktrees []Worktree
		current   Worktree
	)
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if current.Path != "" {
				worktrees = append(worktrees, current)
				current = Worktree{}
			}
			continue
		}
		switch {
		case strings.HasPrefix(line, "worktree "):
			current.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "HEAD "):
			current.Head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			ref := strings.TrimPrefix(line, "branch ")
			current.Branch = strings.TrimPrefix(ref, "refs/heads/")
		case line == "locked":
			current.Locked = true
		}
	}
	// Handle last record if no trailing blank line.
	if current.Path != "" {
		worktrees = append(worktrees, current)
	}
	return worktrees
}

// Prune runs `git worktree prune -v` to remove stale worktree entries.
func (m *Manager) Prune(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "worktree", "prune", "-v")
	cmd.Dir = m.Root
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree prune: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Status returns the current status of a worktree by branch name.
// Combines `git worktree list --porcelain` (for locked) with
// `git -C <path> status --porcelain` (for dirty/clean).
func (m *Manager) Status(ctx context.Context, branch string) (Worktree, error) {
	all, err := m.List(ctx)
	if err != nil {
		return Worktree{}, err
	}
	var wt Worktree
	found := false
	for _, w := range all {
		if w.Branch == branch || strings.HasSuffix(w.Branch, "/"+branch) {
			wt = w
			found = true
			break
		}
	}
	if !found {
		return Worktree{}, fmt.Errorf("worktree not found for branch: %s", branch)
	}

	// Run git status --porcelain to detect dirty state.
	cmd := exec.CommandContext(ctx, "git", "-C", wt.Path, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return Worktree{}, fmt.Errorf("git status --porcelain: %s: %w", strings.TrimSpace(string(out)), err)
	}
	wt.Dirty = len(strings.TrimSpace(string(out))) > 0

	return wt, nil
}
