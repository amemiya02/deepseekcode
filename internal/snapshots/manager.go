// Package snapshots implements pre-edit file rollback per docs/design.md §8.4.
//
// Before any tool that mutates files, the agent calls Take(sessionID,
// stepIdx, paths). Each affected file is copied into:
//
//	.deepseek/snapshots/<sessionID>/<stepIdx>/<base64-of-relpath>
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
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Manager owns the on-disk snapshot directory tree. Construct with New.
//
// All exported mutating operations (Take/Undo/Prune) hold mu so concurrent
// callers cannot interleave reads of the step-dir listing with removals.
type Manager struct {
	mu   sync.Mutex
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
	m.mu.Lock()
	defer m.mu.Unlock()

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
				// Write a tombstone durably. A truncated tombstone would
				// make Undo delete the wrong (or no) path, so it gets the
				// same temp+fsync+rename treatment as a real snapshot.
				if err := writeFileDurable(dst+".absent", []byte(abs), 0o644); err == nil {
					count++
				}
				continue
			}
			return count, fmt.Errorf("open %s: %w", abs, err)
		}
		if err := copyFileDurable(dst, src, 0o644); err != nil {
			_ = src.Close()
			return count, fmt.Errorf("snapshot %s: %w", abs, err)
		}
		_ = src.Close()
		count++
	}
	// Flush the step dir so its newly-created snapshot/tombstone entries
	// survive a crash before the OS would otherwise flush the directory.
	syncDir(stepDir)
	return count, nil
}

// Undo restores the most recent N step snapshots for a session, in
// reverse chronological order. Returns the number of files restored.
func (m *Manager) Undo(sessionID string, n int) (int, error) {
	if n <= 0 {
		n = 1
	}
	m.mu.Lock()
	defer m.mu.Unlock()

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
	m.mu.Lock()
	defer m.mu.Unlock()

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
	m.mu.Lock()
	defer m.mu.Unlock()

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
	// Restores are durable too: a half-restored working-tree file is just
	// as corrupt as a half-taken snapshot, and /undo is the user's last
	// line of defense, so it must not itself leave a truncated file.
	return copyFileDurable(dst, in, 0o644)
}

// copyFileDurable streams src into dst via a sibling temp file, fsyncs the
// temp file, then atomically renames it over dst. An interrupted write can
// therefore only leave a stray "*.tmp-*" file next to dst — never a
// truncated dst that a later restore would treat as complete. The
// containing directory is fsynced so the rename itself is durable.
func copyFileDurable(dst string, src io.Reader, perm os.FileMode) error {
	dir := filepath.Dir(dst)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(dst)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// On any failure path before the successful rename, remove the temp
	// file so a crash recovery never finds a half-written sibling that
	// could be mistaken for content.
	defer func() {
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := io.Copy(tmp, src); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return err
	}
	tmpName = "" // rename succeeded; nothing to clean up.
	syncDir(dir)
	return nil
}

// writeFileDurable writes the whole byte slice to dst via the same
// temp+fsync+atomic-rename dance copyFileDurable uses.
func writeFileDurable(dst string, data []byte, perm os.FileMode) error {
	return copyFileDurable(dst, bytes.NewReader(data), perm)
}

// syncDir best-effort fsyncs a directory so that just-created or just-renamed
// entries are durable. Failure to open/sync a directory (some platforms and
// filesystems do not support it) is non-fatal: the data file was already
// fsynced, so at worst the directory entry is recreated on next write.
func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = d.Sync()
	_ = d.Close()
}
