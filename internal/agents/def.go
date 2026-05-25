package agents

import (
	"strings"
)

// AgentDef is a sub-agent definition loaded from .deepseek/agent/<name>.md.
type AgentDef struct {
	Name        string   // relative path minus .md (filled by Load)
	Description string   // frontmatter "description"
	Mode        string   // frontmatter "mode": "subagent" (default) or "plan"
	Model       string   // frontmatter "model" (empty = inherit parent)
	Tools       []string // frontmatter "tools": comma-separated whitelist; nil = inherit parent full set
	Prompt      string   // body after frontmatter (trimmed)
	Path        string   // absolute path (filled by Load)
}

// ParseAgent parses a .md file into an AgentDef. If the content starts
// with "---\n", the block between the first and second "---" is parsed
// as frontmatter; otherwise the entire content becomes Prompt.
func ParseAgent(content string) (AgentDef, error) {
	if content == "" {
		return AgentDef{}, nil
	}

	body := content
	if strings.HasPrefix(content, "---\n") || strings.HasPrefix(content, "---\r\n") {
		rest := content[4:] // skip first "---\n"
		rest = strings.ReplaceAll(rest, "\r\n", "\n")
		idx := strings.Index(rest, "\n---\n")
		if idx >= 0 {
			fm := rest[:idx]
			body = rest[idx+5:] // skip "\n---\n"
			return parseFrontmatter(fm, body)
		}
		// Only one "---" → treat entire content as prompt.
	}

	return AgentDef{Mode: "subagent", Prompt: strings.TrimSpace(body)}, nil
}

func parseFrontmatter(fm, body string) (AgentDef, error) {
	var d AgentDef
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
			d.Description = val
		case "mode":
			d.Mode = val
		case "model":
			d.Model = val
		case "tools":
			d.Tools = splitTools(val)
		}
	}
	if d.Mode == "" {
		d.Mode = "subagent"
	}
	d.Prompt = strings.TrimSpace(body)
	return d, nil
}

func splitTools(val string) []string {
	if val == "" {
		return nil
	}
	parts := strings.Split(val, ",")
	var out []string
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
