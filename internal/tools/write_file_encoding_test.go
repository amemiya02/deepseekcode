package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/textenc"
)

// TestWriteFilePreservesExistingGBK verifies that overwriting an
// existing GBK file re-encodes the new content to GBK.
func TestWriteFilePreservesExistingGBK(t *testing.T) {
	dir := t.TempDir()
	raw, err := os.ReadFile("../textenc/testdata/gbk.txt")
	if err != nil {
		t.Skip("textenc testdata not built")
	}
	path := filepath.Join(dir, "g.txt")
	must(t, os.WriteFile(path, raw, 0o644))

	// Write new content (UTF-8) to the existing GBK file.
	wf := &WriteFile{CWD: dir}
	args := mustJSON(t, map[string]any{
		"path":    path,
		"content": "你好，世界\nnew content\n",
	})
	res, err := wf.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("write failed: %s", res.Content)
	}

	// Verify: on-disk bytes still detect as a CJK encoding (Detect returns
	// GB18030 for GBK content since GB18030 is the safe superset).
	out, _ := os.ReadFile(path)
	enc := textenc.Detect(out)
	if enc != textenc.GBK && enc != textenc.GB18030 {
		t.Fatalf("write changed encoding from GBK to %v", enc)
	}
	// Verify: decoded content has the new text.
	decoded := string(textenc.Decode(out, textenc.GBK))
	if decoded != "你好，世界\nnew content\n" {
		t.Fatalf("content mismatch after re-encode: %q", decoded)
	}
}

// TestWriteFileNewFileIsUTF8 verifies that writing to a new (absent)
// file produces UTF-8, not some other charset.
func TestWriteFileNewFileIsUTF8(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.txt")

	wf := &WriteFile{CWD: dir}
	args := mustJSON(t, map[string]any{
		"path":    path,
		"content": "你好，世界\nhello\n",
	})
	res, err := wf.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("write failed: %s", res.Content)
	}

	out, _ := os.ReadFile(path)
	if textenc.Detect(out) != textenc.UTF8 {
		t.Fatalf("new file should be UTF-8, got %v", textenc.Detect(out))
	}
	if string(out) != "你好，世界\nhello\n" {
		t.Fatalf("content mismatch: %q", out)
	}
}

// TestWriteFilePreservesUTF16LE verifies that overwriting a UTF-16LE
// file re-encodes to UTF-16LE.
func TestWriteFilePreservesUTF16LE(t *testing.T) {
	dir := t.TempDir()
	raw, err := os.ReadFile("../textenc/testdata/utf16le.txt")
	if err != nil {
		t.Skip("textenc testdata not built")
	}
	path := filepath.Join(dir, "u.txt")
	must(t, os.WriteFile(path, raw, 0o644))

	wf := &WriteFile{CWD: dir}
	args := mustJSON(t, map[string]any{
		"path":    path,
		"content": "你好\nworld\n",
	})
	res, err := wf.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("write failed: %s", res.Content)
	}

	out, _ := os.ReadFile(path)
	if textenc.Detect(out) != textenc.UTF16LE {
		t.Fatalf("write changed encoding from UTF-16LE to %v", textenc.Detect(out))
	}
	decoded := string(textenc.Decode(out, textenc.UTF16LE))
	if decoded != "你好\nworld\n" {
		t.Fatalf("content mismatch: %q", decoded)
	}
}
