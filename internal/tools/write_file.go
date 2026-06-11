package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/amemiya02/deepseekcode/internal/textenc"
)

// WriteFile creates or overwrites a file. Snapshotting is the caller's
// responsibility — the agent invokes the snapshots manager before each
// edit/write tool call.
type WriteFile struct {
	MaxBytes int64  // default 5_242_880 (5 MiB); 0 means use default
	CWD      string // project root for path safety; empty means os.Getwd
	// Tracker, when set, refuses to overwrite an existing file that was never
	// read or that changed on disk since the read (T3.2). A new-file create
	// (path absent on disk) always passes. nil disables the guard.
	Tracker *FileTracker
}

func (WriteFile) Name() string { return "write_file" }

func (WriteFile) Description() string {
	return "Write content to a file, creating parent directories as needed. " +
		"Overwrites the file if it exists. Use edit_file for surgical string-replace " +
		"changes; use write_file for new files or full rewrites. " +
		"The agent automatically snapshots affected files before this tool runs."
}

func (WriteFile) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Absolute or relative path to write.",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Full file content to write.",
			},
		},
		"required": []string{"path", "content"},
	})
}

func (WriteFile) AffectedPaths(args json.RawMessage) []string {
	var p struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(args, &p)
	if p.Path == "" {
		return nil
	}
	return []string{p.Path}
}

func (w WriteFile) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("invalid args: %v", err), nil
	}
	if p.Path == "" {
		return Errf("path is required"), nil
	}

	cwd := w.CWD
	if cwd == "" {
		cwd = "."
	}

	// Refuse to write through a symlink (check raw path).
	if fi, err := os.Lstat(p.Path); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return Errf("refuse to write through symlink: %s. Remove the symlink first or write to the real path directly.", p.Path), nil
	}

	checkedPath, err := ResolveAndCheck(p.Path, cwd)
	if err != nil {
		return Errf("%v", err), nil
	}
	p.Path = checkedPath

	// Read-before-write freshness guard (T3.2): overwriting an existing file
	// that was never read (or that changed since the read) risks clobbering
	// content the agent never saw. A brand-new file passes (CheckFresh returns
	// nil when the path is absent on disk).
	if err := w.Tracker.CheckFresh(p.Path); err != nil {
		return Errf("%v", err), nil
	}

	maxBytes := w.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 5 * 1024 * 1024 // 5 MiB default
	}
	if int64(len(p.Content)) > maxBytes {
		return Errf("content too large (%d bytes; cap is %d). Split into smaller writes or use bash.",
			len(p.Content), maxBytes), nil
	}

	if dir := filepath.Dir(p.Path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return Result{}, fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}

	// Write atomically via tempfile + rename so a crashing process can't
	// leave a half-written file behind.
	//
	// If the file already exists with a non-UTF8 encoding, re-encode the
	// new UTF-8 content to that charset so the file stays consistent.
	// New files (absent on disk) are written as UTF-8 unchanged.
	writeData := []byte(p.Content)
	if existing, err := os.ReadFile(p.Path); err == nil {
		if enc := textenc.Detect(existing); enc != textenc.UTF8 {
			writeData = textenc.Encode(p.Content, enc)
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(p.Path), ".dsc-write-*")
	if err != nil {
		return Result{}, fmt.Errorf("creating tempfile: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(writeData); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return Result{}, fmt.Errorf("writing tempfile: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return Result{}, err
	}
	if err := os.Rename(tmpName, p.Path); err != nil {
		_ = os.Remove(tmpName)
		return Result{}, fmt.Errorf("rename %s -> %s: %w", tmpName, p.Path, err)
	}
	// Stamp the just-written state so a follow-up write/edit in the same
	// session sees it as fresh (T3.2).
	w.Tracker.RecordWrite(p.Path)

	content := fmt.Sprintf("wrote %d bytes to %s", len(p.Content), p.Path)
	if len(p.Content) > 1024*1024 {
		content += fmt.Sprintf("\nwarning: large write (%d bytes). Consider whether this content could be split or trimmed.",
			len(p.Content))
	}
	return Result{Content: content}, nil
}
