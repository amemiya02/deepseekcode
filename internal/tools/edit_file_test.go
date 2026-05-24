package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditFileBasic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	must(t, os.WriteFile(path, []byte("hello world\nfoo bar\nhello world\n"), 0o644))

	args := mustJSON(t, map[string]any{
		"path":       path,
		"old_string": "foo bar",
		"new_string": "baz qux",
	})
	res, err := (&EditFile{CWD: dir}).Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %s", res.Content)
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "baz qux") {
		t.Fatalf("content not updated: %q", got)
	}
}

func TestEditFileRefusesAmbiguous(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "amb.txt")
	must(t, os.WriteFile(path, []byte("x\nx\n"), 0o644))

	args := mustJSON(t, map[string]any{
		"path":       path,
		"old_string": "x",
		"new_string": "y",
	})
	res, err := (&EditFile{CWD: dir}).Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatalf("expected ambiguity error; got: %s", res.Content)
	}
}

func TestEditFileReplaceAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "amb.txt")
	must(t, os.WriteFile(path, []byte("x\nx\n"), 0o644))

	args := mustJSON(t, map[string]any{
		"path":        path,
		"old_string":  "x",
		"new_string":  "y",
		"replace_all": true,
	})
	res, err := (&EditFile{CWD: dir}).Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "y\ny\n" {
		t.Fatalf("got %q", got)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
