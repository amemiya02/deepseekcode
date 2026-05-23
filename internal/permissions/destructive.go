package permissions

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// DestructivePatterns are the bash invocations that trigger the Duet
// Pro Validator (see docs/design.md §11.2). They run regardless of the
// permissions allowlist — even an auto-approved `git push` still gets
// pro to weigh in.
//
// Patterns are RE2 regexes. Users add to the list via
// [duet].extra_destructive_patterns in config.
var DestructivePatterns = []string{
	`^\s*rm(\s|$)`,
	`^\s*rm\s+-r`,
	`^\s*rm\s+-f`,
	`^\s*git\s+push(\s|$)`,
	`^\s*git\s+reset\s+--hard`,
	`^\s*git\s+checkout\s+\.`,
	`^\s*git\s+clean\s+-f`,
	`^\s*curl\s+(?:-[A-Za-z]+\s+)*-X\s+(POST|PUT|DELETE|PATCH)`,
	`^\s*kubectl\s+delete`,
	`^\s*kubectl\s+apply\s+-f`,
	`^\s*terraform\s+apply`,
	`^\s*terraform\s+destroy`,
	`^\s*(psql|mysql|sqlite3).*\b(DROP|DELETE|TRUNCATE)\b`,
	`^\s*(npm|pnpm|yarn)\s+publish`,
	`^\s*docker\s+push`,
}

// IsDestructiveBash returns true if the command matches any destructive
// pattern (built-in or user-configured).
func IsDestructiveBash(command string, extra []string) bool {
	m := defaultMatcher()
	if len(extra) > 0 {
		m = newMatcher(append([]string(nil), DestructivePatterns...), extra)
	}
	return m.Match(command)
}

// IsDestructivePath returns true if writing to path should be considered
// destructive (outside cwd, inside .git, matching secret patterns).
// Used by the Duet to decide whether a write/edit tool needs pro to
// weigh in even if standard permissions would auto-allow it.
//
// (The standard permissions tier *also* prompts for these paths; the
// Duet adds Pro validation in addition to the permission prompt.)
func IsDestructivePath(path, cwd string, secretPatterns []string) bool {
	if path == "" {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return true // can't tell, be safe
	}
	if matchesGitOrSecret(abs, secretPatterns) {
		return true
	}
	cwdAbs, err := filepath.Abs(cwd)
	if err != nil {
		return true
	}
	rel, err := filepath.Rel(cwdAbs, abs)
	if err != nil {
		return true
	}
	if strings.HasPrefix(rel, "..") || rel == ".." {
		return true
	}
	return false
}

// IsDestructiveToolCall returns true if the given tool call (other than
// bash) is destructive enough to invoke the Duet validator.
func IsDestructiveToolCall(toolName string, args json.RawMessage, cwd string, secretPatterns []string) bool {
	switch toolName {
	case "write_file", "edit_file":
		var a struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return true
		}
		return IsDestructivePath(a.Path, cwd, secretPatterns)
	default:
		return false
	}
}

func matchesGitOrSecret(abs string, secretPatterns []string) bool {
	parts := strings.Split(filepath.ToSlash(abs), "/")
	for _, part := range parts {
		if part == ".git" {
			return true
		}
	}
	base := filepath.Base(abs)
	for _, pat := range secretPatterns {
		if ok, _ := filepath.Match(pat, base); ok {
			return true
		}
	}
	return false
}

type matcher struct {
	res []*regexp.Regexp
}

var (
	defaultMatcherOnce sync.Once
	defaultMatcherVal  *matcher
)

func defaultMatcher() *matcher {
	defaultMatcherOnce.Do(func() {
		defaultMatcherVal = newMatcher(DestructivePatterns, nil)
	})
	return defaultMatcherVal
}

func newMatcher(builtin, extra []string) *matcher {
	patterns := append(append([]string(nil), builtin...), extra...)
	all := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			continue // skip invalid user-supplied patterns silently
		}
		all = append(all, re)
	}
	return &matcher{res: all}
}

func (m *matcher) Match(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	for _, re := range m.res {
		if re.MatchString(cmd) {
			return true
		}
	}
	return false
}
