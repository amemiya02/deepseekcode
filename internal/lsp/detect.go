package lsp

import (
	"os"
	"os/exec"
	"path/filepath"
)

// ServerInfo describes a recommended LSP server for a detected language.
type ServerInfo struct {
	Name    string   // e.g. "gopls"
	Command string   // binary name
	Args    []string // extra args
}

// DetectServers scans cwd for project files and returns LSP server
// recommendations whose binaries are in PATH.
func DetectServers(cwd string) []ServerInfo {
	var out []ServerInfo

	checks := []struct {
		file    string
		command string
		name    string
		args    []string
	}{
		{"go.mod", "gopls", "gopls", nil},
		{"Cargo.toml", "rust-analyzer", "rust-analyzer", nil},
		{"tsconfig.json", "typescript-language-server", "typescript-language-server", []string{"--stdio"}},
		{"pyproject.toml", "pylsp", "pylsp", nil},
		{"requirements.txt", "pylsp", "pylsp", nil},
	}

	seen := map[string]bool{}
	for _, c := range checks {
		if seen[c.name] {
			continue
		}
		if _, err := os.Stat(filepath.Join(cwd, c.file)); err != nil {
			continue
		}
		if _, err := exec.LookPath(c.command); err != nil {
			continue
		}
		out = append(out, ServerInfo{
			Name:    c.name,
			Command: c.command,
			Args:    c.args,
		})
		seen[c.name] = true
	}

	return out
}
