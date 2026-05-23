package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Ls lists immediate children of a directory. No recursion — that's
// what glob is for. Returns a `type indicator + name` per line so the
// model can distinguish files, directories, and symlinks at a glance.
type Ls struct{}

func (Ls) Name() string { return "ls" }

func (Ls) Description() string {
	return "List immediate children of a directory (no recursion). " +
		"Returns one entry per line, prefixed with [d] for directory, [f] for file, " +
		"[l] for symlink. For recursive discovery use glob; for searching content use grep."
}

func (Ls) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Directory to list. Default: current directory.",
			},
		},
		"required": []string{"path"},
	})
}

func (Ls) IsReadOnly() bool { return true }

func (Ls) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("invalid args: %v", err), nil
	}
	if p.Path == "" {
		p.Path = "."
	}
	entries, err := os.ReadDir(p.Path)
	if err != nil {
		return Errf("ls %s: %v", p.Path, err), nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		var prefix string
		switch {
		case e.Type()&os.ModeSymlink != 0:
			prefix = "[l]"
		case e.IsDir():
			prefix = "[d]"
		default:
			prefix = "[f]"
		}
		names = append(names, fmt.Sprintf("%s %s", prefix, e.Name()))
	}
	sort.Strings(names)
	if len(names) == 0 {
		return Result{Content: fmt.Sprintf("(empty: %s)", p.Path)}, nil
	}
	return Result{Content: strings.Join(names, "\n")}, nil
}
