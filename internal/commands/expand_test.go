package commands

import (
	"context"
	"fmt"
	"testing"
)

func mockRunner(_ context.Context, cmd string) (string, error) {
	if cmd == "fail" {
		return "", fmt.Errorf("boom")
	}
	return "OUTPUT:" + cmd, nil
}

func mockReadFile(path string) (string, error) {
	if path == "missing" {
		return "", fmt.Errorf("not found")
	}
	return "FILE:" + path, nil
}

func TestExpand_PositionalArgs(t *testing.T) {
	got, _ := Expand(context.Background(), "fix $1 in $2", []string{"bug", "auth.go"}, nil, nil)
	if got != "fix bug in auth.go" {
		t.Errorf("got %q", got)
	}
}

func TestExpand_HighestAbsorbs(t *testing.T) {
	got, _ := Expand(context.Background(), "review $1", []string{"a", "b", "c"}, nil, nil)
	if got != "review a b c" {
		t.Errorf("got %q", got)
	}
}

func TestExpand_Arguments(t *testing.T) {
	got, _ := Expand(context.Background(), "run $ARGUMENTS now", []string{"x", "y"}, nil, nil)
	if got != "run x y now" {
		t.Errorf("got %q", got)
	}
}

func TestExpand_NoPlaceholderAppends(t *testing.T) {
	got, _ := Expand(context.Background(), "summarize", []string{"pkg/x"}, nil, nil)
	if got != "summarize\npkg/x" {
		t.Errorf("got %q", got)
	}
}

func TestExpand_ShellSuccess(t *testing.T) {
	got, _ := Expand(context.Background(), "diff:\n!`git diff`", nil, mockRunner, nil)
	if got != "diff:\nOUTPUT:git diff" {
		t.Errorf("got %q", got)
	}
}

func TestExpand_ShellFailure(t *testing.T) {
	got, _ := Expand(context.Background(), "!`fail`", nil, mockRunner, nil)
	if got != "[command failed: boom]" {
		t.Errorf("got %q", got)
	}
}

func TestExpand_FileSuccess(t *testing.T) {
	got, _ := Expand(context.Background(), "see @readme", nil, nil, mockReadFile)
	if got != "see FILE:readme" {
		t.Errorf("got %q", got)
	}
}

func TestExpand_FileMissing(t *testing.T) {
	got, _ := Expand(context.Background(), "see @missing", nil, nil, mockReadFile)
	if got != "see @missing" {
		t.Errorf("got %q", got)
	}
}

func TestExpand_EmptyArgs(t *testing.T) {
	got, _ := Expand(context.Background(), "hello $1 world", nil, nil, nil)
	if got != "hello  world" {
		t.Errorf("got %q", got)
	}
}

func TestExpand_DollarZero(t *testing.T) {
	// $0 is out of range (args start at $1) → empty string.
	got, _ := Expand(context.Background(), "a $0 b", []string{"x"}, nil, nil)
	if got != "a  b" {
		t.Errorf("got %q", got)
	}
}

func TestExpand_NoPlaceholderNoArgs(t *testing.T) {
	got, _ := Expand(context.Background(), "just template", nil, nil, nil)
	if got != "just template" {
		t.Errorf("got %q", got)
	}
}

func TestExpand_WordAtHostNotExpanded(t *testing.T) {
	// "user@host" — @ preceded by word char, must NOT be treated as @file.
	got, _ := Expand(context.Background(), "send to user@host", nil, nil, mockReadFile)
	if got != "send to user@host" {
		t.Errorf("got %q, should preserve user@host", got)
	}
}

func TestExpand_FileAtStartOfLine(t *testing.T) {
	got, _ := Expand(context.Background(), "@readme", nil, nil, mockReadFile)
	if got != "FILE:readme" {
		t.Errorf("got %q", got)
	}
}
