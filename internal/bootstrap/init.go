package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
)

// InitOptions controls the behaviour of dsc init.
type InitOptions struct {
	Force bool   // overwrite existing files
	CWD   string // project root; empty means os.Getwd
}

// Run creates DEEPSEEK.md and .deepseek/config.toml in the project root.
// It detects the project language and writes appropriate build/test commands
// into the templates. Existing files are skipped unless Force is true.
func Run(opts InitOptions) error {
	cwd := opts.CWD
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("getwd: %w", err)
		}
	}

	lang := detectLang(cwd)

	// Write .deepseek/config.toml
	configDir := filepath.Join(cwd, ".deepseek")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", configDir, err)
	}
	configPath := filepath.Join(configDir, "config.toml")
	if err := writeFile(configPath, configToml(lang), opts.Force); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	// Write .deepseek/.gitignore
	gitignorePath := filepath.Join(configDir, ".gitignore")
	if err := writeFile(gitignorePath, "*\n", opts.Force); err != nil {
		// Non-fatal: .gitignore may already exist.
		_ = err
	}

	// Write DEEPSEEK.md
	mdPath := filepath.Join(cwd, "DEEPSEEK.md")
	if err := writeFile(mdPath, deepseekMD(lang), opts.Force); err != nil {
		return fmt.Errorf("write DEEPSEEK.md: %w", err)
	}

	fmt.Fprintf(os.Stderr, "dsc init: wrote %s, %s\n", configPath, mdPath)
	return nil
}

func writeFile(path, content string, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			fmt.Fprintf(os.Stderr, "  skip %s (exists; use --force to overwrite)\n", path)
			return nil
		}
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
