package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/textenc"
)

// TestEditFilePreservesGBK verifies that editing a GBK file preserves
// the original charset: the on-disk result should still be GBK, the
// edited content should be correct, and the unedited CJK should survive.
func TestEditFilePreservesGBK(t *testing.T) {
	dir := t.TempDir()
	raw, err := os.ReadFile("../textenc/testdata/gbk.txt")
	if err != nil {
		t.Skip("textenc testdata not built")
	}
	path := filepath.Join(dir, "g.txt")
	must(t, os.WriteFile(path, raw, 0o644))

	// Edit "hello" → "world" (ASCII portion of the GBK file).
	args := mustJSON(t, map[string]any{
		"path":       path,
		"old_string": "hello",
		"new_string": "world",
	})
	ef := &EditFile{CWD: dir}
	res, err := ef.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("edit failed: %s", res.Content)
	}

	// Verify: on-disk bytes still detect as a CJK encoding (Detect returns
	// GB18030 for GBK content since GB18030 is the safe superset).
	out, _ := os.ReadFile(path)
	enc := textenc.Detect(out)
	if enc != textenc.GBK && enc != textenc.GB18030 {
		t.Fatalf("edit changed encoding from GBK to %v", enc)
	}
	// Verify: decoded content has both the replacement and the original CJK.
	decoded := string(textenc.Decode(out, textenc.GBK))
	if !strings.Contains(decoded, "world") {
		t.Fatalf("replacement not applied: %q", decoded)
	}
	if !strings.Contains(decoded, "你好，世界") {
		t.Fatalf("CJK content corrupted by edit: %q", decoded)
	}
}

// TestEditFilePreservesUTF16LE verifies that editing a UTF-16LE file
// preserves the charset and round-trips correctly.
func TestEditFilePreservesUTF16LE(t *testing.T) {
	dir := t.TempDir()
	raw, err := os.ReadFile("../textenc/testdata/utf16le.txt")
	if err != nil {
		t.Skip("textenc testdata not built")
	}
	path := filepath.Join(dir, "u.txt")
	must(t, os.WriteFile(path, raw, 0o644))

	args := mustJSON(t, map[string]any{
		"path":       path,
		"old_string": "hello",
		"new_string": "world",
	})
	ef := &EditFile{CWD: dir}
	res, err := ef.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("edit failed: %s", res.Content)
	}

	out, _ := os.ReadFile(path)
	if textenc.Detect(out) != textenc.UTF16LE {
		t.Fatalf("edit changed encoding from UTF-16LE to %v", textenc.Detect(out))
	}
	decoded := string(textenc.Decode(out, textenc.UTF16LE))
	if !strings.Contains(decoded, "world") {
		t.Fatalf("replacement not applied: %q", decoded)
	}
	if !strings.Contains(decoded, "你好") {
		t.Fatalf("CJK content corrupted: %q", decoded)
	}
}
