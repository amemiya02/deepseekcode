package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ResolveAndCheck returns the canonicalized absolute path (symlinks resolved).
// It rejects paths that escape cwd via symlink traversal. Both cwd and the
// target path are fully resolved before comparison.
func ResolveAndCheck(raw, cwd string) (string, error) {
	cwdAbs, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Errorf("resolve cwd: %w", err)
	}
	cwdReal, err := filepath.EvalSymlinks(cwdAbs)
	if err != nil {
		return "", fmt.Errorf("resolve cwd symlinks: %w", err)
	}

	abs, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("resolve path %s: %w", raw, err)
	}
	real, err := evalExistingPath(abs)
	if err != nil {
		return "", fmt.Errorf("resolve path symlinks %s: %w", raw, err)
	}

	if !isWithin(cwdReal, real) {
		return "", fmt.Errorf("path escapes cwd via symlink: %s resolves to %s (outside %s)", raw, real, cwdReal)
	}
	return real, nil
}

// evalExistingPath resolves symlinks on the deepest existing ancestor of
// abs and appends the non-existing remainder. This handles paths that do
// not exist yet (e.g. write_file creating a new file).
func evalExistingPath(abs string) (string, error) {
	real, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return real, nil
	}
	existing := abs
	for {
		parent := filepath.Dir(existing)
		if parent == existing {
			return abs, nil
		}
		existing = parent
		if resolved, e2 := filepath.EvalSymlinks(existing); e2 == nil {
			rel, _ := filepath.Rel(existing, abs)
			return filepath.Join(resolved, rel), nil
		}
	}
}

// isWithin reports whether path is equal to or resides inside root, after
// resolving both to their canonical forms. On Windows, comparison is
// case-insensitive.
func isWithin(root, path string) bool {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		root = strings.ToLower(root)
		path = strings.ToLower(path)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != ".." && !filepath.IsAbs(rel))
}
