package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReadFileDecodesCJK verifies that a GBK file is decoded to UTF-8
// and its CJK content is visible to the model.
func TestReadFileDecodesCJK(t *testing.T) {
	dir := t.TempDir()
	raw, err := os.ReadFile("../textenc/testdata/gbk.txt")
	if err != nil {
		t.Skip("textenc testdata not built; run: make textenc-fixtures")
	}
	path := filepath.Join(dir, "g.txt")
	must(t, os.WriteFile(path, raw, 0o644))

	r := ReadFile{CWD: dir}
	res, err := r.Execute(context.Background(), mustJSON(t, map[string]any{"path": "g.txt"}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "你好，世界") {
		t.Fatalf("expected decoded CJK content, got: %q", res.Content)
	}
	if strings.Contains(res.Content, "binary file") {
		t.Fatalf("GBK file misreported as binary")
	}
}

// TestReadFileUTF16NotBinary verifies that a UTF-16LE file is NOT
// reported as binary and its content is decoded to UTF-8.  This is
// the critical regression test for the NUL-check ordering fix: the
// raw NUL-byte check must happen AFTER encoding detection, not before.
func TestReadFileUTF16NotBinary(t *testing.T) {
	dir := t.TempDir()
	raw, err := os.ReadFile("../textenc/testdata/utf16le.txt")
	if err != nil {
		t.Skip("textenc testdata not built; run: make textenc-fixtures")
	}
	path := filepath.Join(dir, "u.txt")
	must(t, os.WriteFile(path, raw, 0o644))

	r := ReadFile{CWD: dir}
	res, err := r.Execute(context.Background(), mustJSON(t, map[string]any{"path": "u.txt"}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("UTF-16 misreported as binary: %s", res.Content)
	}
	if !strings.Contains(res.Content, "你好") {
		t.Fatalf("UTF-16 content not decoded: %q", res.Content)
	}
}

// TestReadFileUTF16BE decodes a UTF-16BE file.
func TestReadFileUTF16BE(t *testing.T) {
	dir := t.TempDir()
	raw, err := os.ReadFile("../textenc/testdata/utf16be.txt")
	if err != nil {
		t.Skip("textenc testdata not built")
	}
	path := filepath.Join(dir, "u.txt")
	must(t, os.WriteFile(path, raw, 0o644))

	r := ReadFile{CWD: dir}
	res, err := r.Execute(context.Background(), mustJSON(t, map[string]any{"path": "u.txt"}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("UTF-16BE misreported as binary: %s", res.Content)
	}
	if !strings.Contains(res.Content, "你好") {
		t.Fatalf("UTF-16BE content not decoded: %q", res.Content)
	}
}

// TestReadFileGB18030 decodes a GB18030 file.
func TestReadFileGB18030(t *testing.T) {
	dir := t.TempDir()
	raw, err := os.ReadFile("../textenc/testdata/gb18030.txt")
	if err != nil {
		t.Skip("textenc testdata not built")
	}
	path := filepath.Join(dir, "g.txt")
	must(t, os.WriteFile(path, raw, 0o644))

	r := ReadFile{CWD: dir}
	res, err := r.Execute(context.Background(), mustJSON(t, map[string]any{"path": "g.txt"}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "你好，世界") {
		t.Fatalf("GB18030 content not decoded: %q", res.Content)
	}
}

// TestReadFileUTF8Unchanged verifies that plain UTF-8 files still take
// the original streaming path (zero behaviour change).
func TestReadFileUTF8Unchanged(t *testing.T) {
	dir := t.TempDir()
	raw, err := os.ReadFile("../textenc/testdata/utf8.txt")
	if err != nil {
		t.Skip("textenc testdata not built")
	}
	path := filepath.Join(dir, "u.txt")
	must(t, os.WriteFile(path, raw, 0o644))

	r := ReadFile{CWD: dir}
	res, err := r.Execute(context.Background(), mustJSON(t, map[string]any{"path": "u.txt"}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "你好，世界") {
		t.Fatalf("UTF-8 content missing: %q", res.Content)
	}
}

// TestReadFileGenuineBinaryStillRejected verifies that a file with
// actual NUL bytes in its UTF-8 content is still reported as binary.
func TestReadFileGenuineBinaryStillRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bin.dat")
	must(t, os.WriteFile(path, []byte{0x00, 0x01, 0x02, 0x03}, 0o644))

	r := ReadFile{CWD: dir}
	res, _ := r.Execute(context.Background(), mustJSON(t, map[string]any{"path": "bin.dat"}))
	if !res.IsError || !strings.Contains(res.Content, "binary file") {
		t.Fatalf("expected binary rejection, got: %q", res.Content)
	}
}
