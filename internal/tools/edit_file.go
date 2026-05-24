package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// EditFile is a string-replace editor. It refuses ambiguous matches so
// the model can't accidentally edit the wrong occurrence. If the user
// wants every occurrence replaced they must pass replace_all=true.
//
// This mirrors Claude Code's Edit tool semantics, which is now the de
// facto standard for agent edits.
type EditFile struct {
	CWD string // project root for path safety; empty means os.Getwd
}

func (EditFile) Name() string { return "edit_file" }

func (EditFile) Description() string {
	return "Surgically edit a file by replacing exactly one occurrence of old_string " +
		"with new_string. Fails if old_string appears zero times or more than once " +
		"(use replace_all=true to replace every occurrence). " +
		"Preserve exact indentation and surrounding context when constructing old_string. " +
		"For new files or full rewrites, use write_file instead. " +
		"The agent automatically snapshots affected files before this tool runs."
}

func (EditFile) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Absolute or relative path of the file to edit.",
			},
			"old_string": map[string]any{
				"type":        "string",
				"description": "Exact string to find. Must match exactly, including whitespace.",
			},
			"new_string": map[string]any{
				"type":        "string",
				"description": "Replacement string. Empty string deletes the match.",
			},
			"replace_all": map[string]any{
				"type":        "boolean",
				"description": "If true, replace every occurrence. Otherwise old_string must match exactly once.",
			},
		},
		"required": []string{"path", "old_string", "new_string"},
	})
}

func (EditFile) AffectedPaths(args json.RawMessage) []string {
	var p struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(args, &p)
	if p.Path == "" {
		return nil
	}
	return []string{p.Path}
}

func (e EditFile) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		Path       string `json:"path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("invalid args: %v", err), nil
	}
	if p.Path == "" {
		return Errf("path is required"), nil
	}
	if p.OldString == "" {
		return Errf("old_string must be non-empty"), nil
	}
	if p.OldString == p.NewString {
		return Errf("old_string and new_string are identical; nothing to do"), nil
	}

	cwd := e.CWD
	if cwd == "" {
		cwd = "."
	}
	checkedPath, err := ResolveAndCheck(p.Path, cwd)
	if err != nil {
		return Errf("%v", err), nil
	}
	p.Path = checkedPath

	b, err := os.ReadFile(p.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Errf("file not found: %s (use write_file to create)", p.Path), nil
		}
		return Result{}, fmt.Errorf("reading %s: %w", p.Path, err)
	}
	content := string(b)

	count := strings.Count(content, p.OldString)
	if count == 0 {
		return Errf("old_string not found in %s. The exact substring (including whitespace) must match.", p.Path), nil
	}
	if count > 1 && !p.ReplaceAll {
		return Errf("old_string appears %d times in %s. Provide a longer, unique snippet or set replace_all=true.",
			count, p.Path), nil
	}

	var updated string
	if p.ReplaceAll {
		updated = strings.ReplaceAll(content, p.OldString, p.NewString)
	} else {
		updated = strings.Replace(content, p.OldString, p.NewString, 1)
	}

	if err := atomicWriteFile(p.Path, []byte(updated)); err != nil {
		return Result{}, err
	}
	return Result{Content: fmt.Sprintf("edited %s (%d replacement%s)",
		p.Path, count, plural(count))}, nil
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
