package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Grep searches file contents for a regex. Shells out to ripgrep when
// available (much faster, respects .gitignore for free), falls back to
// a stdlib walker otherwise.
type Grep struct{}

func (Grep) Name() string { return "grep" }

func (Grep) Description() string {
	return "Search file contents for a regular expression (RE2 syntax). " +
		"Optional path narrows the search; optional glob_filter restricts file names (e.g. '*.go'). " +
		"Optional case_insensitive matches without regard to case. " +
		"Returns up to 200 matches as 'path:line:content'. " +
		"Use this for finding usages, definitions, error messages, configuration keys."
}

func (Grep) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "RE2 regular expression to search for.",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Optional path (file or directory). Default: current directory.",
			},
			"glob_filter": map[string]any{
				"type":        "string",
				"description": "Optional glob to filter filenames (e.g. '*.go').",
			},
			"case_insensitive": map[string]any{
				"type":        "boolean",
				"description": "If true, match without regard to case.",
			},
		},
		"required": []string{"pattern"},
	})
}

func (Grep) IsReadOnly() bool { return true }

func (Grep) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		Pattern    string `json:"pattern"`
		Path       string `json:"path"`
		GlobFilter string `json:"glob_filter"`
		CaseInsens bool   `json:"case_insensitive"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("invalid args: %v", err), nil
	}
	if p.Pattern == "" {
		return Errf("pattern is required"), nil
	}
	if p.Path == "" {
		p.Path = "."
	}

	if rgPath, err := exec.LookPath("rg"); err == nil {
		return grepRipgrep(ctx, rgPath, p.Pattern, p.Path, p.GlobFilter, p.CaseInsens)
	}
	return grepStdlib(ctx, p.Pattern, p.Path, p.GlobFilter, p.CaseInsens)
}

const grepMaxMatches = 200

func grepRipgrep(ctx context.Context, rgPath, pattern, path, glob string, ci bool) (Result, error) {
	args := []string{
		"--no-heading",
		"--with-filename",
		"--line-number",
		"--color=never",
		"--max-count=20",
		"-e", pattern,
	}
	if glob != "" {
		args = append(args, "--glob", glob)
	}
	if ci {
		args = append(args, "--ignore-case")
	}
	args = append(args, path)

	cmd := exec.CommandContext(ctx, rgPath, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		// ripgrep exits 1 when there are no matches; treat that as success.
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1 {
			return Result{Content: fmt.Sprintf("(no matches for %q under %s)", pattern, path)}, nil
		}
		return Result{}, fmt.Errorf("ripgrep: %w (%s)", err, out.String())
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) > grepMaxMatches {
		lines = append(lines[:grepMaxMatches],
			fmt.Sprintf("... (%d more matches, refine the pattern)", len(lines)-grepMaxMatches))
	}
	return Result{Content: strings.Join(lines, "\n")}, nil
}

func grepStdlib(ctx context.Context, pattern, path, glob string, ci bool) (Result, error) {
	re, err := compileRegex(pattern, ci)
	if err != nil {
		return Errf("invalid regex: %v", err), nil
	}
	var matches []string
	err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == ".venv" {
				return fs.SkipDir
			}
			return nil
		}
		if glob != "" {
			ok, _ := filepath.Match(glob, d.Name())
			if !ok {
				return nil
			}
		}
		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		lineNo := 0
		fileMatches := 0
		for sc.Scan() {
			lineNo++
			line := sc.Text()
			if re.MatchString(line) {
				matches = append(matches, fmt.Sprintf("%s:%d:%s", p, lineNo, line))
				fileMatches++
				if fileMatches >= 20 || len(matches) >= grepMaxMatches+1 {
					break
				}
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}
		return nil
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		return Result{}, fmt.Errorf("walk: %w", err)
	}
	sort.Strings(matches)
	if len(matches) == 0 {
		return Result{Content: fmt.Sprintf("(no matches for %q under %s)", pattern, path)}, nil
	}
	if len(matches) > grepMaxMatches {
		matches = append(matches[:grepMaxMatches],
			fmt.Sprintf("... (%d more matches, refine the pattern)", len(matches)-grepMaxMatches))
	}
	return Result{Content: strings.Join(matches, "\n")}, nil
}

func compileRegex(pat string, ci bool) (*regexp.Regexp, error) {
	if ci {
		pat = "(?i)" + pat
	}
	return regexp.Compile(pat)
}
