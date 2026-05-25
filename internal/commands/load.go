package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxCommandFileSize = 64 * 1024 // 64 KiB

// Load scans cwd/.deepseek/command and home/.deepseek/command for *.md
// files and returns a name→Command map. Same-name commands: cwd wins.
// Missing directories are silently skipped.
func Load(cwd, home string) (map[string]Command, error) {
	if cwd == "" {
		return nil, fmt.Errorf("Load: empty cwd")
	}
	roots := []string{cwd}
	if home != "" && home != cwd {
		roots = append(roots, home)
	}

	seen := map[string]bool{}
	out := map[string]Command{}

	for _, root := range roots {
		dir := filepath.Join(root, ".deepseek", "command")
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // missing dir → skip
		}
		loadDir(dir, dir, entries, seen, out)
	}
	return out, nil
}

func loadDir(baseDir, dir string, entries []os.DirEntry, seen map[string]bool, out map[string]Command) {
	for _, e := range entries {
		full := filepath.Join(dir, e.Name())
		if e.IsDir() {
			if strings.HasPrefix(e.Name(), ".") {
				continue // skip hidden dirs
			}
			sub, err := os.ReadDir(full)
			if err != nil {
				continue
			}
			loadDir(baseDir, full, sub, seen, out)
			continue
		}
		if !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		info, err := e.Info()
		if err != nil || info.Size() > maxCommandFileSize {
			continue
		}
		rel, _ := filepath.Rel(baseDir, full)
		name := strings.TrimSuffix(rel, ".md")
		name = filepath.ToSlash(name)
		if seen[name] {
			continue
		}
		data, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		cmd, err := ParseCommand(string(data))
		if err != nil {
			continue
		}
		cmd.Name = name
		cmd.Path, _ = filepath.Abs(full)
		out[name] = cmd
		seen[name] = true
	}
}
