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

// TestEditFileCRLFRoundTrip verifies that a CRLF file edited with an LF
// old_string (the common drift when a model strips \r) still matches via the
// CRLF->LF normalization, and that the written bytes are converted BACK to
// CRLF so the file's line ending is preserved.
func TestEditFileCRLFRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "crlf.txt")
	must(t, os.WriteFile(path, []byte("alpha\r\nbeta\r\ngamma\r\n"), 0o644))

	args := mustJSON(t, map[string]any{
		"path":       path,
		"old_string": "beta", // LF/no-CR snippet from the model
		"new_string": "BETA",
	})
	res, err := (&EditFile{CWD: dir}).Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "alpha\r\nBETA\r\ngamma\r\n" {
		t.Fatalf("CRLF not preserved; got %q", got)
	}
}

// TestEditFileFuzzyTrailingSpace exercises the fuzzy cascade end-to-end through
// Execute: an old_string with trailing-space drift matches the original line
// and writes the original (untrailing-spaced) bytes around the edit.
func TestEditFileFuzzyTrailingSpace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fuzzy_exec.go")
	must(t, os.WriteFile(path, []byte("func f() {\n\tx := 1\n\treturn x\n}\n"), 0o644))

	args := mustJSON(t, map[string]any{
		"path":       path,
		"old_string": "x := 1  \n\treturn x", // trailing spaces on line 1
		"new_string": "\tx := 2\n\treturn x",
	})
	res, err := (&EditFile{CWD: dir}).Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "func f() {\n\tx := 2\n\treturn x\n}\n" {
		t.Fatalf("fuzzy edit produced wrong bytes; got %q", got)
	}
}

// TestEditFileAmbiguousMessage verifies the distinct ambiguity branch fires for
// a fuzzy multi-match (two trim-equal regions, no unique resolution) with the
// model-actionable "matched multiple locations" guidance.
func TestEditFileAmbiguousMessage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "amb_fuzzy.txt")
	must(t, os.WriteFile(path, []byte("  a = 1\nmid\n  a = 1\n"), 0o644))

	args := mustJSON(t, map[string]any{
		"path":       path,
		"old_string": "a = 1  ", // trailing-space drift, matches both indented lines
		"new_string": "a = 2",
	})
	res, err := (&EditFile{CWD: dir}).Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatalf("expected ambiguity error; got: %s", res.Content)
	}
	if !strings.Contains(res.Content, "matched multiple locations") {
		t.Fatalf("expected 'matched multiple locations' guidance; got: %s", res.Content)
	}
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
