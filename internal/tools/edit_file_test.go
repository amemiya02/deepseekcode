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

func TestEditFileFuzzyHint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fuzzy.txt")
	content := "line one\nline two\nline three\nline four\n"
	must(t, os.WriteFile(path, []byte(content), 0o644))

	t.Run("close match gives hint", func(t *testing.T) {
		args := mustJSON(t, map[string]any{
			"path":       path,
			"old_string": "line one\nline two\nline threeX",
			"new_string": "replaced",
		})
		res, err := (&EditFile{CWD: dir}).Execute(context.Background(), args)
		if err != nil {
			t.Fatal(err)
		}
		if !res.IsError {
			t.Fatalf("expected error result; got: %s", res.Content)
		}
		if !strings.Contains(res.Content, "Closest match at line 1") {
			t.Fatalf("expected fuzzy hint; got: %s", res.Content)
		}
		if !strings.Contains(res.Content, "distance 1") {
			t.Fatalf("expected distance 1; got: %s", res.Content)
		}
	})

	t.Run("too far gives plain not found", func(t *testing.T) {
		args := mustJSON(t, map[string]any{
			"path":       path,
			"old_string": "completely\ndifferent\ncontent\nhere\nnow",
			"new_string": "replaced",
		})
		res, err := (&EditFile{CWD: dir}).Execute(context.Background(), args)
		if err != nil {
			t.Fatal(err)
		}
		if !res.IsError {
			t.Fatalf("expected error result; got: %s", res.Content)
		}
		if strings.Contains(res.Content, "Closest match") {
			t.Fatalf("should not give fuzzy hint for distant match; got: %s", res.Content)
		}
		if !strings.Contains(res.Content, "old_string not found") {
			t.Fatalf("expected plain not-found; got: %s", res.Content)
		}
	})

	t.Run("exact match still works", func(t *testing.T) {
		args := mustJSON(t, map[string]any{
			"path":       path,
			"old_string": "line two",
			"new_string": "line 2",
		})
		res, err := (&EditFile{CWD: dir}).Execute(context.Background(), args)
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", res.Content)
		}
	})
}
