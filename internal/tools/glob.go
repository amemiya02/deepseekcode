package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// Glob lists files matching a doublestar pattern. We use a small
// in-package matcher to keep the dependency tree thin; the patterns we
// support are: `*` (any chars except /), `?` (one char except /), `**`
// (any chars including /), and literal chars.
type Glob struct{}

func (Glob) Name() string { return "glob" }

func (Glob) Description() string {
	return "Find files by glob pattern. Supports * (any chars in a path component), " +
		"? (one char in a path component), and ** (any number of path components). " +
		"Examples: '**/*.go', 'cmd/*/main.go', 'docs/**/*.md'. " +
		"Optional cwd narrows the search root (default: current directory). " +
		"Returns sorted paths. Use grep to search within file contents."
}

func (Glob) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Glob pattern (supports * ? **).",
			},
			"cwd": map[string]any{
				"type":        "string",
				"description": "Optional root directory for the search.",
			},
		},
		"required": []string{"pattern"},
	})
}

func (Glob) IsReadOnly() bool { return true }

func (Glob) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		Pattern string `json:"pattern"`
		Cwd     string `json:"cwd"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("invalid args: %v", err), nil
	}
	if p.Pattern == "" {
		return Errf("pattern is required"), nil
	}
	root := p.Cwd
	if root == "" {
		root = "."
	}

	var matches []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries silently
		}
		// Skip noisy directories that almost never want to be in results.
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == ".venv" ||
				name == "__pycache__" || name == "dist" || name == "target" {
				return fs.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}
		// Match against the pattern, evaluating relative to root.
		ok, _ := doublestarMatch(p.Pattern, rel)
		if ok {
			matches = append(matches, path)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	})
	if err != nil {
		if err == context.Canceled || err == context.DeadlineExceeded {
			return Errf("cancelled"), nil
		}
		return Result{}, fmt.Errorf("walking %s: %w", root, err)
	}
	sort.Strings(matches)

	if len(matches) == 0 {
		return Result{Content: fmt.Sprintf("(no matches for pattern %q under %s)", p.Pattern, root)}, nil
	}
	const cap = 1000
	if len(matches) > cap {
		matches = append(matches[:cap], fmt.Sprintf("... (%d more, refine the pattern)", len(matches)-cap))
	}
	return Result{Content: strings.Join(matches, "\n")}, nil
}

// doublestarMatch implements a small subset of doublestar matching.
// Returns (matched, error). The pattern is split on `/` and walked
// against the path segments with `**` consuming any number of segments.
func doublestarMatch(pattern, path string) (bool, error) {
	pp := strings.Split(filepath.ToSlash(pattern), "/")
	ps := strings.Split(filepath.ToSlash(path), "/")
	return matchSegs(pp, ps), nil
}

func matchSegs(pat, path []string) bool {
	for len(pat) > 0 && len(path) > 0 {
		if pat[0] == "**" {
			// Try matching ** as zero or more segments.
			rest := pat[1:]
			if len(rest) == 0 {
				return true
			}
			for i := 0; i <= len(path); i++ {
				if matchSegs(rest, path[i:]) {
					return true
				}
			}
			return false
		}
		ok, _ := filepath.Match(pat[0], path[0])
		if !ok {
			return false
		}
		pat = pat[1:]
		path = path[1:]
	}
	// Consume trailing ** greedily.
	for len(pat) > 0 && pat[0] == "**" {
		pat = pat[1:]
	}
	return len(pat) == 0 && len(path) == 0
}
