package permissions

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/amemiya02/deepseekcode/internal/tools"
)

// DestructivePatterns documents the bash invocations that the Duet Pro
// Validator treats as destructive (see docs/design.md §11.2).
// These are now handled by tools.ClassifyBash; this list is kept for
// reference and for the extra-patterns fallback.
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

// IsDestructiveBash returns true if ClassifyBash classifies the command as
// BashDestructive, or if the command matches any user-configured extra
// patterns.
func IsDestructiveBash(command string, extra []string) bool {
	if tools.ClassifyBash(command) == tools.BashDestructive {
		return true
	}
	if len(extra) > 0 {
		return extraMatcher(extra).Match(command)
	}
	return false
}

// IsDestructivePath returns true if writing to path should be considered
// destructive (outside cwd, inside .git, matching secret patterns).
func IsDestructivePath(path, cwd string, secretPatterns []string) bool {
	if path == "" {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return true
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
	extraMatcherCache struct {
		mu    sync.Mutex
		cache map[string]*matcher
	}
)

func extraMatcher(extra []string) *matcher {
	key := strings.Join(extra, "\x00")
	extraMatcherCache.mu.Lock()
	defer extraMatcherCache.mu.Unlock()
	if extraMatcherCache.cache == nil {
		extraMatcherCache.cache = make(map[string]*matcher)
	}
	if m, ok := extraMatcherCache.cache[key]; ok {
		return m
	}
	m := newMatcherFromPatterns(extra)
	extraMatcherCache.cache[key] = m
	return m
}

func newMatcherFromPatterns(patterns []string) *matcher {
	all := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			continue
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
