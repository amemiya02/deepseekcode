package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Lang holds detected project language info.
type Lang struct {
	Name    string // "go", "node", "python", "rust", or "unknown"
	Build   string
	Test    string
	PkgMgr  string
	Lint    string
	Modules []string // e.g. ["go"], ["npm", "pnpm", "yarn"]
}

func detectLang(cwd string) Lang {
	// Priority: go.mod > Cargo.toml > pyproject.toml > package.json
	if exists(filepath.Join(cwd, "go.mod")) {
		return Lang{
			Name:   "go",
			Build:  "go build ./...",
			Test:   "go test -race ./...",
			PkgMgr: "go",
			Lint:   "go vet ./...",
		}
	}
	if exists(filepath.Join(cwd, "Cargo.toml")) {
		return Lang{
			Name:   "rust",
			Build:  "cargo build",
			Test:   "cargo test",
			PkgMgr: "cargo",
			Lint:   "cargo clippy",
		}
	}
	if exists(filepath.Join(cwd, "pyproject.toml")) {
		return Lang{
			Name:   "python",
			Build:  "pip install -e .",
			Test:   "pytest",
			PkgMgr: "pip",
			Lint:   "ruff check .",
		}
	}
	if exists(filepath.Join(cwd, "package.json")) {
		pkgMgr := "npm"
		if exists(filepath.Join(cwd, "pnpm-lock.yaml")) {
			pkgMgr = "pnpm"
		} else if exists(filepath.Join(cwd, "yarn.lock")) {
			pkgMgr = "yarn"
		}
		return Lang{
			Name:   "node",
			Build:  pkgMgr + " run build",
			Test:   pkgMgr + " test",
			PkgMgr: pkgMgr,
			Lint:   pkgMgr + " run lint",
		}
	}
	return Lang{
		Name:   "unknown",
		Build:  "make",
		Test:   "make test",
		PkgMgr: "make",
		Lint:   "make lint",
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func deepseekMD(l Lang) string {
	var b strings.Builder
	b.WriteString("# DEEPSEEK.md\n\n")
	b.WriteString("Project instructions for the DeepSeek coding agent.\n\n")

	b.WriteString("## Commands\n\n")
	fmt.Fprintf(&b, "- Build: `%s`\n", l.Build)
	fmt.Fprintf(&b, "- Test: `%s`\n", l.Test)
	if l.Lint != "" {
		fmt.Fprintf(&b, "- Lint: `%s`\n", l.Lint)
	}

	b.WriteString("\n## Conventions\n\n")
	switch l.Name {
	case "go":
		b.WriteString("- Run `gofmt -s -w .` before committing\n")
		b.WriteString("- Tests live in the same package (`*_test.go`)\n")
		b.WriteString("- No CGO dependencies\n")
	case "node":
		b.WriteString("- Use the project's package manager for all installs\n")
		b.WriteString("- Tests co-located with source files\n")
	case "python":
		b.WriteString("- Use a virtual environment\n")
		b.WriteString("- Format with ruff before committing\n")
	case "rust":
		b.WriteString("- Run `cargo fmt` before committing\n")
		b.WriteString("- Clippy should pass with no warnings\n")
	default:
		b.WriteString("- Follow existing project patterns\n")
	}

	b.WriteString("\n## Structure\n\n")
	b.WriteString("<!-- Describe key directories and their purpose here -->\n")

	return b.String()
}

func configToml(l Lang) string {
	var b strings.Builder
	b.WriteString("# .deepseek/config.toml — deepseekcode project config\n\n")

	b.WriteString("[api]\n")
	b.WriteString("# key = \"\"  # or set DEEPSEEK_API_KEY in env\n")
	b.WriteString("# base_url = \"https://api.deepseek.com\"\n\n")

	b.WriteString("[defaults]\n")
	switch l.Name {
	case "go":
		b.WriteString("model = \"deepseek-v4\"\n")
	default:
		b.WriteString("# model = \"deepseek-v4\"\n")
	}
	b.WriteString("thinking = true\n")
	b.WriteString("# reasoning_effort = \"max\"  # low|medium|high|max\n\n")

	b.WriteString("[duet]\n")
	b.WriteString("enabled = true\n\n")

	b.WriteString("[permissions]\n")
	b.WriteString("# allow_bash = [\"git status *\", \"go test *\"]\n")
	b.WriteString("# secret_path_patterns = [\".env*\", \"*.pem\", \"*.key\"]\n\n")

	b.WriteString("[sessions]\n")
	b.WriteString("ttl_days = 30\n")
	b.WriteString("snapshot_keep = 10\n")

	return b.String()
}
