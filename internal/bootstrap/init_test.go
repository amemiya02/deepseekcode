package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectLang(t *testing.T) {
	cases := []struct {
		setup func(dir string)
		want  string
	}{
		{func(d string) { _ = os.WriteFile(filepath.Join(d, "go.mod"), nil, 0o644) }, "go"},
		{func(d string) { _ = os.WriteFile(filepath.Join(d, "Cargo.toml"), nil, 0o644) }, "rust"},
		{func(d string) { _ = os.WriteFile(filepath.Join(d, "pyproject.toml"), nil, 0o644) }, "python"},
		{func(d string) { _ = os.WriteFile(filepath.Join(d, "package.json"), nil, 0o644) }, "node"},
		{func(d string) {}, "unknown"},
	}

	for _, c := range cases {
		dir := t.TempDir()
		c.setup(dir)
		got := detectLang(dir)
		if got.Name != c.want {
			t.Errorf("detectLang after setup → %q; want %q", got.Name, c.want)
		}
	}
}

func TestDetectLangNodePkgMgr(t *testing.T) {
	cases := []struct {
		extra string
		want  string
	}{
		{"pnpm-lock.yaml", "pnpm"},
		{"yarn.lock", "yarn"},
		{"", "npm"},
	}
	for _, c := range cases {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, "package.json"), nil, 0o644)
		if c.extra != "" {
			_ = os.WriteFile(filepath.Join(dir, c.extra), nil, 0o644)
		}
		got := detectLang(dir)
		if got.PkgMgr != c.want {
			t.Errorf("pkgMgr with %s → %q; want %q", c.extra, got.PkgMgr, c.want)
		}
	}
}

func TestDetectLangPriority(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), nil, 0o644)
	_ = os.WriteFile(filepath.Join(dir, "package.json"), nil, 0o644)
	got := detectLang(dir)
	if got.Name != "go" {
		t.Errorf("go.mod should win over package.json; got %q", got.Name)
	}
}

func TestRunCreatesFiles(t *testing.T) {
	dir := t.TempDir()

	// Write go.mod so detection works.
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\ngo 1.22\n"), 0o644)

	err := Run(InitOptions{CWD: dir})
	if err != nil {
		t.Fatal(err)
	}

	// Check DEEPSEEK.md exists and has build command.
	md, err := os.ReadFile(filepath.Join(dir, "DEEPSEEK.md"))
	if err != nil {
		t.Fatal("DEEPSEEK.md not created:", err)
	}
	if !strings.Contains(string(md), "go build") {
		t.Errorf("DEEPSEEK.md missing build command; got:\n%s", md)
	}

	// Check .deepseek/config.toml exists.
	cfg, err := os.ReadFile(filepath.Join(dir, ".deepseek", "config.toml"))
	if err != nil {
		t.Fatal(".deepseek/config.toml not created:", err)
	}
	if !strings.Contains(string(cfg), "[api]") {
		t.Errorf("config.toml missing [api]; got:\n%s", cfg)
	}
}

func TestRunSkipExisting(t *testing.T) {
	dir := t.TempDir()

	// Pre-create DEEPSEEK.md with custom content.
	_ = os.WriteFile(filepath.Join(dir, "DEEPSEEK.md"), []byte("# custom"), 0o644)

	err := Run(InitOptions{CWD: dir})
	if err != nil {
		t.Fatal(err)
	}

	md, _ := os.ReadFile(filepath.Join(dir, "DEEPSEEK.md"))
	if string(md) != "# custom" {
		t.Error("existing DEEPSEEK.md should not be overwritten without --force")
	}
}

func TestRunForceOverwrite(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "DEEPSEEK.md"), []byte("# old"), 0o644)

	err := Run(InitOptions{CWD: dir, Force: true})
	if err != nil {
		t.Fatal(err)
	}

	md, _ := os.ReadFile(filepath.Join(dir, "DEEPSEEK.md"))
	if string(md) == "# old" {
		t.Error("--force should overwrite existing DEEPSEEK.md")
	}
}

func TestTemplateContent(t *testing.T) {
	lang := Lang{Name: "go", Build: "go build ./...", Test: "go test -race ./...", Lint: "go vet ./..."}
	md := deepseekMD(lang)
	if !strings.Contains(md, "gofmt") {
		t.Error("Go template should mention gofmt")
	}

	lang = Lang{Name: "node", Build: "npm run build", Test: "npm test", Lint: "npm run lint"}
	md = deepseekMD(lang)
	if !strings.Contains(md, "package manager") {
		t.Error("Node template should mention package manager")
	}
}
