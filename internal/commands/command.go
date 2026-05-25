package commands

import (
	"strconv"
	"strings"
)

// Command represents a user-defined slash command loaded from a .md file.
type Command struct {
	Name        string // derived from relative path (filled by Load)
	Description string // frontmatter "description"
	Agent       string // frontmatter "agent" (parsed, not active — TODO Phase 21)
	Model       string // frontmatter "model" (active)
	Subtask     bool   // frontmatter "subtask" (parsed, not active — TODO Phase 21)
	Template    string // body after frontmatter (trimmed)
	Path        string // absolute path (filled by Load)
}

// ParseCommand parses a .md file into a Command. If the content starts
// with "---\n", the block between the first and second "---" is parsed
// as frontmatter; otherwise the entire content becomes Template.
func ParseCommand(content string) (Command, error) {
	if content == "" {
		return Command{}, nil
	}

	body := content
	if strings.HasPrefix(content, "---\n") || strings.HasPrefix(content, "---\r\n") {
		rest := content[4:] // skip first "---\n"
		// Normalize \r\n
		rest = strings.ReplaceAll(rest, "\r\n", "\n")
		idx := strings.Index(rest, "\n---\n")
		if idx >= 0 {
			fm := rest[:idx]
			body = rest[idx+5:] // skip "\n---\n"
			return parseFrontmatter(fm, body)
		}
		// Only one "---" → treat entire content as template.
	}

	return Command{Template: strings.TrimSpace(body)}, nil
}

func parseFrontmatter(fm, body string) (Command, error) {
	var c Command
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
		val = unquote(val)

		switch key {
		case "description":
			c.Description = val
		case "agent":
			c.Agent = val
		case "model":
			c.Model = val
		case "subtask":
			c.Subtask, _ = strconv.ParseBool(val)
		}
	}
	c.Template = strings.TrimSpace(body)
	return c, nil
}

func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
