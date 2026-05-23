// Package snapshots implements pre-edit file rollback per docs/design.md §8.4.
//
// Before any tool that mutates files, the agent calls Take(sessionID,
// stepIdx, paths). Each affected file is copied into:
//
//   .deepseek/snapshots/<sessionID>/<stepIdx>/<base64-of-relpath>
//
// /undo restores the most-recent step's snapshots; /undo N restores N
// steps. Prune trims to the most recent K sessions.
//
// Snapshots never include directories — only files we observe being
// touched by name. Bash commands hand AffectedPaths==nil to the agent,
// so destructive bash is handled by the Duet validator and the
// permission prompt, not by pre-snapshot.
package snapshots

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Manager owns the on-disk snapshot directory tree. Construct with New.
type Manager struct {
	root string // typically ./.deepseek/snapshots
}

// New returns a Manager rooted at .deepseek/snapshots under cwd.
// Passing a non-empty rootOverride is for tests.
func New(rootOverride string) *Manager {
	if rootOverride == "" {
		rootOverride = filepath.Join(".deepseek", "snapshots")
	}
	return &Manager{root: rootOverride}
}

// Take snapshots each existing path in `paths` into the session/step
// directory. Missing files are recorded as "absent" tombstones so /undo
// knows to remove them when reverting.
//
// Returns the number of snapshots actually written.
func (m *Manager) Take(sessionID string, stepIdx int, paths []string) (int, error) {
	if len(paths) == 0 {
		return 0, nil
	}
	stepDir, err := m.stepDir(sessionID, stepIdx)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(stepDir, 0o755); err != nil {
		return 0, fmt.Errorf("mkdir %s: %w", stepDir, err)
	}

	count := 0
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		dst := filepath.Join(stepDir, encodePath(abs))

		src, err := os.Open(abs)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				// Write a tombstone alongside.
				if err := os.WriteFile(dst+".absent", []byte(abs), 0o644); err == nil {
					count++
				}
				continue
			}
			return count, fmt.Errorf("open %s: %w", abs, err)
		}
		out, err := os.Create(dst)
		if err != nil {
			_ = src.Close()
			return count, fmt.Errorf("create %s: %w", dst, err)
		}
		if _, err := io.Copy(out, src); err != nil {
			_ = src.Close()
			_ = out.Close()
			return count, fmt.Errorf("copy %s: %w", abs, err)
		}
		_ = src.Close()
		if err := out.Close(); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// Undo restores the most recent N step snapshots for a session, in
// reverse chronological order. Returns the number of files restored.
func (m *Manager) Undo(sessionID string, n int) (int, error) {
	if n <= 0 {
		n = 1
	}
	sessDir := filepath.Join(m.root, sessionID)
	steps, err := listStepDirs(sessDir)
	if err != nil {
		return 0, err
	}
	if len(steps) == 0 {
		return 0, errors.New("no snapshots for session")
	}

	restored := 0
	limit := n
	if limit > len(steps) {
		limit = len(steps)
	}
	for i := len(steps) - 1; i >= len(steps)-limit; i-- {
		stepDir := filepath.Join(sessDir, steps[i])
		entries, err := os.ReadDir(stepDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			full := filepath.Join(stepDir, name)
			if strings.HasSuffix(name, ".absent") {
				absent, err := os.ReadFile(full)
				if err == nil {
					_ = os.Remove(strings.TrimSpace(string(absent)))
					restored++
				}
				continue
			}
			abs := decodePath(name)
			if abs == "" {
				continue
			}
			if err := restoreFile(full, abs); err == nil {
				restored++
			}
		}
		// Consume this step dir so a future Undo doesn't see it again.
		_ = os.RemoveAll(stepDir)
	}
	return restored, nil
}

// Prune removes snapshot dirs for sessions not in `keepSessionIDs`.
// Typical usage: keep the most recent 30 session IDs from the store.
func (m *Manager) Prune(keepSessionIDs []string) (int, error) {
	keep := make(map[string]struct{}, len(keepSessionIDs))
	for _, id := range keepSessionIDs {
		keep[id] = struct{}{}
	}
	entries, err := os.ReadDir(m.root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	removed := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, ok := keep[e.Name()]; ok {
			continue
		}
		if err := os.RemoveAll(filepath.Join(m.root, e.Name())); err == nil {
			removed++
		}
	}
	return removed, nil
}

// HasSnapshots reports whether the session has any unconsumed snapshots.
func (m *Manager) HasSnapshots(sessionID string) bool {
	steps, _ := listStepDirs(filepath.Join(m.root, sessionID))
	return len(steps) > 0
}

func (m *Manager) stepDir(sessionID string, stepIdx int) (string, error) {
	if sessionID == "" {
		return "", errors.New("empty session id")
	}
	return filepath.Join(m.root, sessionID, fmt.Sprintf("%06d", stepIdx)), nil
}

func listStepDirs(sessDir string) ([]string, error) {
	entries, err := os.ReadDir(sessDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var dirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		dirs = append(dirs, e.Name())
	}
	sort.Strings(dirs)
	return dirs, nil
}

// encodePath produces a single filename-safe token from an absolute
// path. We use URL-safe base64 (no padding) so paths can round-trip on
// every filesystem we target.
func encodePath(abs string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(abs))
}

func decodePath(token string) string {
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return ""
	}
	return string(b)
}

func restoreFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
