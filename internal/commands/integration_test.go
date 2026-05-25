package commands

import (
	"context"
	"path/filepath"
	"testing"
)

func TestIntegration_LoadExpand(t *testing.T) {
	dir := t.TempDir()
	// Template uses the canonical !`cmd` syntax and $ARGUMENTS.
	content := "---\ndescription: commit helper\nmodel: deepseek-v4-pro\n---\ncommit and push\n## DIFF\n!`echo hi`\n$ARGUMENTS"
	write(filepath.Join(dir, ".deepseek", "command", "commit.md"), content)

	cmds, err := Load(dir, "")
	if err != nil {
		t.Fatal(err)
	}

	cmd, ok := cmds["commit"]
	if !ok {
		t.Fatal("commit command not found")
	}
	if cmd.Model != "deepseek-v4-pro" {
		t.Errorf("Model = %q, want deepseek-v4-pro", cmd.Model)
	}
	if cmd.Description != "commit helper" {
		t.Errorf("Description = %q", cmd.Description)
	}

	args := Tokenize("a b")
	runner := func(_ context.Context, c string) (string, error) {
		return "MOCK:" + c, nil
	}
	got, err := Expand(context.Background(), cmd.Template, args, runner, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "commit and push\n## DIFF\nMOCK:echo hi\na b"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestIntegration_CwdOverridesHome(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	write(filepath.Join(cwd, ".deepseek", "command", "x.md"), "---\nmodel: pro\n---\ncwd version")
	write(filepath.Join(home, ".deepseek", "command", "x.md"), "---\nmodel: flash\n---\nhome version")

	cmds, err := Load(cwd, home)
	if err != nil {
		t.Fatal(err)
	}
	if cmds["x"].Model != "pro" {
		t.Errorf("cwd should override home, got Model=%q", cmds["x"].Model)
	}
}

func TestIntegration_NestedCommand(t *testing.T) {
	dir := t.TempDir()
	write(filepath.Join(dir, ".deepseek", "command", "git", "sync.md"), "sync $1")

	cmds, err := Load(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	cmd, ok := cmds["git/sync"]
	if !ok {
		t.Fatal("git/sync not found")
	}
	got, _ := Expand(context.Background(), cmd.Template, []string{"main"}, nil, nil)
	if got != "sync main" {
		t.Errorf("got %q", got)
	}
}
