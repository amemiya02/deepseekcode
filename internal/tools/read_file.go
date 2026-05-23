package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// ReadFile reads a UTF-8 text file and returns it with cat -n style line
// numbers. Models do better at reasoning about positions when they see
// numbers next to lines.
type ReadFile struct{}

func (ReadFile) Name() string { return "read_file" }

func (ReadFile) Description() string {
	return "Read a UTF-8 text file from the filesystem and return its contents " +
		"with cat -n style line numbers (e.g. '  42\\tcontent'). " +
		"Optional start_line and end_line (1-indexed, inclusive) restrict the range. " +
		"If end_line exceeds the file length, returns up to the last line. " +
		"On binary files or non-UTF8 content, returns an error result describing the situation."
}

func (ReadFile) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Absolute or relative path to the file.",
			},
			"start_line": map[string]any{
				"type":        "integer",
				"description": "Optional 1-indexed inclusive start line.",
			},
			"end_line": map[string]any{
				"type":        "integer",
				"description": "Optional 1-indexed inclusive end line.",
			},
		},
		"required": []string{"path"},
	})
}

func (ReadFile) IsReadOnly() bool { return true }

func (ReadFile) AffectedPaths(args json.RawMessage) []string {
	var p struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(args, &p)
	if p.Path == "" {
		return nil
	}
	return []string{p.Path}
}

func (ReadFile) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("invalid args: %v", err), nil
	}
	if p.Path == "" {
		return Errf("path is required"), nil
	}

	f, err := os.Open(p.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Errf("file not found: %s", p.Path), nil
		}
		return Result{}, fmt.Errorf("opening %s: %w", p.Path, err)
	}
	defer f.Close()

	const maxBytes = 2 * 1024 * 1024 // 2 MiB hard cap; bigger files need a different tool
	stat, err := f.Stat()
	if err == nil && stat.Size() > maxBytes {
		return Errf("file too large (%d bytes; cap is %d). Use grep or a narrower start_line/end_line.",
			stat.Size(), maxBytes), nil
	}

	start := p.StartLine
	end := p.EndLine
	if start < 0 || end < 0 || (end > 0 && start > end) {
		return Errf("invalid line range: start=%d end=%d", start, end), nil
	}
	if start == 0 {
		start = 1
	}

	var out strings.Builder
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNum := 0
	for sc.Scan() {
		lineNum++
		if lineNum < start {
			continue
		}
		if end != 0 && lineNum > end {
			break
		}
		// cat -n format: 6-wide right-aligned line number, tab, content.
		fmt.Fprintf(&out, "%6d\t%s\n", lineNum, sc.Text())
		select {
		case <-ctx.Done():
			return Errf("cancelled at line %d", lineNum), nil
		default:
		}
	}
	if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) {
		return Errf("read error at line %d: %v", lineNum, err), nil
	}
	if lineNum == 0 {
		return Result{Content: fmt.Sprintf("(empty file: %s)", p.Path)}, nil
	}
	if start > lineNum {
		return Errf("start_line=%d is past end of file (%d lines)", start, lineNum), nil
	}
	return Result{Content: out.String()}, nil
}
