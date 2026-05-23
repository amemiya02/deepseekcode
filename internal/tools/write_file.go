package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteFile creates or overwrites a file. Snapshotting is the caller's
// responsibility — the agent invokes the snapshots manager before each
// edit/write tool call.
type WriteFile struct{}

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

func (WriteFile) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
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

	if dir := filepath.Dir(p.Path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return Result{}, fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}

	// Write atomically via tempfile + rename so a crashing process can't
	// leave a half-written file behind.
	tmp, err := os.CreateTemp(filepath.Dir(p.Path), ".dsc-write-*")
	if err != nil {
		return Result{}, fmt.Errorf("creating tempfile: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(p.Content); err != nil {
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

	return Result{Content: fmt.Sprintf("wrote %d bytes to %s", len(p.Content), p.Path)}, nil
}
