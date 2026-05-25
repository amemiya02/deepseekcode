package prompt

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Skill represents a discovered SKILL.md that the model can reference by
// name and open via read_file when needed. Skills are loaded once at
// session start and injected into the static prefix (before the dynamic
// context boundary) for cache stability.
type Skill struct {
	Name        string // frontmatter "name" (required); fallback: first # heading
	Description string // frontmatter "description" (optional)
	Path        string // absolute path to SKILL.md (filled by LoadSkills)
	Body        string // content after frontmatter
}

var headingRe = regexp.MustCompile(`(?m)^#\s+(.+)$`)

// ParseSkill parses a single SKILL.md content.
//   - With "---" frontmatter: extracts name (required, error if missing)
//     and description.
//   - Without frontmatter: uses the first "# heading" as name.
//     Error if no heading found.
//
// Returned Skill.Path is empty; the caller fills it.
func ParseSkill(content string) (Skill, error) {
	if content == "" {
		return Skill{}, fmt.Errorf("skill has neither frontmatter name nor a # heading")
	}

	// Try frontmatter parsing.
	if strings.HasPrefix(content, "---\n") || strings.HasPrefix(content, "---\r\n") {
		rest := content[4:]
		rest = strings.ReplaceAll(rest, "\r\n", "\n")
		idx := strings.Index(rest, "\n---\n")
		if idx >= 0 {
			fm := rest[:idx]
			body := rest[idx+5:]
			return parseSkillFrontmatter(fm, body)
		}
		// Single "---" with no closing — fall through to heading logic.
	}

	// No frontmatter: use first # heading as name.
	m := headingRe.FindStringSubmatch(content)
	if m == nil {
		return Skill{}, fmt.Errorf("skill has neither frontmatter name nor a # heading")
	}
	return Skill{Name: strings.TrimSpace(m[1]), Body: strings.TrimSpace(content)}, nil
}

func parseSkillFrontmatter(fm, body string) (Skill, error) {
	var s Skill
	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(strings.ToLower(parts[0]))
		val := strings.TrimSpace(parts[1])
		val = unquoteSkill(val)
		switch key {
		case "name":
			s.Name = val
		case "description":
			s.Description = val
		}
	}
	if s.Name == "" {
		return Skill{}, fmt.Errorf("missing required frontmatter field: name")
	}
	s.Body = strings.TrimSpace(body)
	return s, nil
}

// skillScanDirs lists directories (relative to cwd then home) to scan for
// SKILL.md files, in priority order. dsc-native dirs come first.
var skillScanDirs = []string{
	".deepseek/skills",
	"skills",
	".opencode/skills",
	".claude/skills",
	".agents/skills",
}

const maxSkillWalkDepth = 8

// LoadSkills scans cross-tool skill directories for **/SKILL.md files,
// parses them, and returns deduplicated skills in discovery order.
// Directories are scanned cwd-first, then home (if different). Missing
// directories are silently skipped. Parse failures are logged and skipped.
func LoadSkills(cwd, home string) ([]Skill, error) {
	if cwd == "" {
		return nil, fmt.Errorf("LoadSkills: empty cwd")
	}
	roots := []string{cwd}
	if home != "" && home != cwd {
		roots = append(roots, home)
	}

	seen := map[string]bool{}
	var out []Skill

	for _, root := range roots {
		for _, rel := range skillScanDirs {
			dir := filepath.Join(root, rel)
			walkSkillDir(dir, dir, 0, seen, &out)
		}
	}
	return out, nil
}

func walkSkillDir(baseDir, dir string, depth int, seen map[string]bool, out *[]Skill) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // missing or unreadable → skip
	}
	// Phase 1: if this directory contains SKILL.md, take it and stop
	// drilling this subtree. (Contract: "hit SKILL.md → stop descent".)
	for _, e := range entries {
		if e.Name() == "SKILL.md" && !e.IsDir() {
			full := filepath.Join(dir, "SKILL.md")
			s, err := loadSkillFile(full)
			if err != nil {
				slog.Warn("skill parse failed", "path", full, "err", err)
				return // malformed skill dir — don't drill further
			}
			s.Path, _ = filepath.Abs(full)
			if !seen[s.Name] {
				seen[s.Name] = true
				*out = append(*out, s)
			}
			return
		}
	}
	// Phase 2: no SKILL.md here → recurse into subdirectories.
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name[0] == '.' {
			continue // skip hidden dirs at all levels
		}
		if depth >= maxSkillWalkDepth {
			continue
		}
		walkSkillDir(baseDir, filepath.Join(dir, name), depth+1, seen, out)
	}
}

func loadSkillFile(path string) (Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, err
	}
	return ParseSkill(string(data))
}

func unquoteSkill(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
