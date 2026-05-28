package agents

import (
	"strconv"
	"strings"
)

// AgentDef is a sub-agent definition loaded from .deepseek/agent/<name>.md.
type AgentDef struct {
	Name        string   // relative path minus .md (filled by Load)
	Description string   // frontmatter "description"
	Mode        string   // frontmatter "mode": "subagent" (default) or "plan"
	Model       string   // frontmatter "model" (empty = inherit parent)
	Tools       []string // frontmatter "tools": comma-separated whitelist; nil = inherit parent full set
	Worktree    bool     // frontmatter "worktree": true|false — isolate in git worktree
	Prompt      string   // body after frontmatter (trimmed)
	Path        string   // absolute path (filled by Load)
	Hidden            bool
	MaxSteps          int
	PermissionRuleset string
	Temperature       *float64
	TopP              *float64
	DefaultAgent      string
}

// AgentProfile is a first-class runtime unit that governs tool tiers,
// permissions, and context policies for an agent session.
type AgentProfile struct {
	Name             string
	Model            string
	ReasoningEffort  string
	SystemOverlay    string
	ToolTiers        []string // "core", "profile", "lazy"
	AllowedTools     []string // explicit tool whitelist; nil = all from tiers
	PermissionPolicy string
	ContextPolicy    string
	CompactionPolicy string
	SubagentPolicy   string
	CachePolicy      string
}

// DefaultProfiles returns the built-in agent profiles.
func DefaultProfiles() map[string]AgentProfile {
	return map[string]AgentProfile{
		"coding-default": {
			Name:            "coding-default",
			Model:           "deepseek-v4-flash",
			ReasoningEffort: "medium",
			ToolTiers:       []string{"core"},
		},
		"explore": {
			Name:            "explore",
			Model:           "deepseek-v4-flash",
			ReasoningEffort: "low",
			ToolTiers:       []string{"core", "profile"},
			AllowedTools: []string{
				"read_file", "glob", "grep", "ls",
				"git_diff", "git_show", "git_blame", "git_log",
				"web_fetch", "web_search",
			},
		},
		"implement": {
			Name:            "implement",
			Model:           "deepseek-v4-flash",
			ReasoningEffort: "high",
			ToolTiers:       []string{"core", "profile"},
			AllowedTools: []string{
				"read_file", "write_file", "edit_file", "apply_patch",
				"bash", "bash_pty", "glob", "grep", "ls", "todo_write",
				"git_diff", "git_show", "git_blame", "git_log",
			},
		},
		"review": {
			Name:            "review",
			Model:           "deepseek-v4-flash",
			ReasoningEffort: "medium",
			ToolTiers:       []string{"core"},
			AllowedTools: []string{
				"read_file", "glob", "grep", "ls",
				"git_diff", "git_show", "git_blame", "git_log",
			},
		},
		"autonomous": {
			Name:            "autonomous",
			Model:           "deepseek-v4-flash",
			ReasoningEffort: "high",
			ToolTiers:       []string{"core", "profile", "lazy"},
			PermissionPolicy: "auto-approve",
			SubagentPolicy:   "allow",
		},
	}
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
		case "worktree":
			d.Worktree = parseBool(val)
		case "hidden":
			d.Hidden = parseBool(val)
		case "max_steps", "maxsteps":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				d.MaxSteps = n
			}
		case "permission_ruleset", "permissions":
			d.PermissionRuleset = val
		case "temperature":
			if f, err := strconv.ParseFloat(val, 64); err == nil {
				d.Temperature = &f
			}
		case "top_p":
			if f, err := strconv.ParseFloat(val, 64); err == nil {
				d.TopP = &f
			}
		case "default_agent":
			d.DefaultAgent = val
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

func parseBool(s string) bool {
	switch strings.ToLower(s) {
	case "true", "yes", "1":
		return true
	default:
		return false
	}
}
