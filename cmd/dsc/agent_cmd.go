package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/agents"
)

func runAgentCommand(cwd string, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("usage: dsc agent list|show|new|validate")
	}
	switch args[0] {
	case "list":
		defs, err := agents.Load(cwd, "")
		if err != nil {
			return "", err
		}
		names := make([]string, 0, len(defs))
		for name := range defs {
			names = append(names, name)
		}
		sort.Strings(names)
		var b strings.Builder
		for _, name := range names {
			d := defs[name]
			if d.Hidden {
				continue
			}
			fmt.Fprintf(&b, "%s\t%s\n", name, d.Description)
		}
		return b.String(), nil
	case "show":
		if len(args) != 2 {
			return "", fmt.Errorf("usage: dsc agent show NAME")
		}
		path := filepath.Join(cwd, ".deepseek", "agent", args[1]+".md")
		body, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return string(body), nil
	case "new":
		if len(args) != 2 {
			return "", fmt.Errorf("usage: dsc agent new NAME")
		}
		agentDir := filepath.Join(cwd, ".deepseek", "agent")
		if err := os.MkdirAll(agentDir, 0o755); err != nil {
			return "", err
		}
		path := filepath.Join(agentDir, args[1]+".md")
		body := fmt.Sprintf("---\ndescription: %s\nmode: subagent\ntools: read_file, grep, git_diff\n---\nYou are the %s subagent. Inspect the repository and return concise findings with file paths.\n", args[1], args[1])
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return "", err
		}
		return "created " + filepath.ToSlash(filepath.Join(".deepseek", "agent", args[1]+".md")) + "\n", nil
	case "validate":
		if err := validateAgentFiles(cwd); err != nil {
			return "", err
		}
		return "agents valid\n", nil
	default:
		return "", fmt.Errorf("unknown agent command %q", args[0])
	}
}

func validateAgentFiles(cwd string) error {
	agentDir := filepath.Join(cwd, ".deepseek", "agent")
	return filepath.WalkDir(agentDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if hasUnclosedFrontmatter(string(body)) {
			return fmt.Errorf("%s: malformed frontmatter", path)
		}
		if _, err := agents.ParseAgent(string(body)); err != nil {
			return err
		}
		return nil
	})
}

func hasUnclosedFrontmatter(s string) bool {
	if !(strings.HasPrefix(s, "---\n") || strings.HasPrefix(s, "---\r\n")) {
		return false
	}
	rest := strings.ReplaceAll(s[4:], "\r\n", "\n")
	return !strings.Contains(rest, "\n---\n")
}
