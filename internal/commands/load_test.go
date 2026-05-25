package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_NestedSubdir(t *testing.T) {
	dir := t.TempDir()
	write(filepath.Join(dir, ".deepseek", "command", "commit.md"), "tmpl commit")
	write(filepath.Join(dir, ".deepseek", "command", "git", "sync.md"), "tmpl sync")

	cmds, err := Load(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if cmds["commit"].Template != "tmpl commit" {
		t.Errorf("commit template = %q", cmds["commit"].Template)
	}
	if cmds["git/sync"].Template != "tmpl sync" {
		t.Errorf("git/sync template = %q", cmds["git/sync"].Template)
	}
}

func TestLoad_CwdOverridesHome(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	write(filepath.Join(cwd, ".deepseek", "command", "x.md"), "---\nmodel: pro\n---\ncwd")
	write(filepath.Join(home, ".deepseek", "command", "x.md"), "---\nmodel: flash\n---\nhome")

	cmds, err := Load(cwd, home)
	if err != nil {
		t.Fatal(err)
	}
	if cmds["x"].Template != "cwd" {
		t.Errorf("cwd should win, got %q", cmds["x"].Template)
	}
	if cmds["x"].Model != "pro" {
		t.Errorf("model = %q, want pro", cmds["x"].Model)
	}
}

func TestLoad_MissingDir(t *testing.T) {
	dir := t.TempDir()
	cmds, err := Load(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 0 {
		t.Errorf("expected empty map, got %d entries", len(cmds))
	}
}

func TestLoad_EmptyCwd(t *testing.T) {
	_, err := Load("", "")
	if err == nil {
		t.Error("expected error for empty cwd")
	}
}

func TestLoad_OversizeFileSkipped(t *testing.T) {
	dir := t.TempDir()
	big := strings.Repeat("x", maxCommandFileSize+1)
	write(filepath.Join(dir, ".deepseek", "command", "big.md"), big)

	cmds, err := Load(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cmds["big"]; ok {
		t.Error("oversize file should be skipped")
	}
}

func write(path, content string) {
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte(content), 0o644)
}
