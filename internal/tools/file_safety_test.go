package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------- T-601: binary detection ----------

func TestReadFileBinaryDetect(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "binary.bin")
	if err := os.WriteFile(path, []byte("text\x00binary"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := &ReadFile{CWD: dir}
	args, _ := json.Marshal(map[string]any{"path": path})
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected IsError=true for binary file")
	}
	if !strings.Contains(res.Content, "NUL byte") {
		t.Errorf("expected NUL byte message, got: %s", res.Content)
	}
	if !strings.Contains(strings.ToLower(res.Content), "consider") {
		t.Errorf("expected 'consider' hint, got: %s", res.Content)
	}
}

func TestReadFileBinaryDetectEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	tool := &ReadFile{CWD: dir}
	args, _ := json.Marshal(map[string]any{"path": path})
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Errorf("empty file should not be flagged as binary: %s", res.Content)
	}
}

func TestReadFileBinaryDetectSmallText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "small.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := &ReadFile{CWD: dir}
	args, _ := json.Marshal(map[string]any{"path": path})
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Errorf("text file should not be flagged as binary: %s", res.Content)
	}
}

// ---------- T-602: MaxReadBytes ----------

func TestReadFileMaxSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	// Write a file larger than the 100-byte limit we set.
	content := strings.Repeat("x", 200)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := &ReadFile{MaxBytes: 100, CWD: dir}
	args, _ := json.Marshal(map[string]any{"path": path})
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error for oversized file")
	}
	if !strings.Contains(res.Content, "file too large") {
		t.Errorf("expected 'file too large', got: %s", res.Content)
	}
}

func TestReadFileMaxSizeDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ok.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	// MaxBytes=0 means use default 5MB — small file should pass.
	tool := &ReadFile{MaxBytes: 0, CWD: dir}
	args, _ := json.Marshal(map[string]any{"path": path})
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Errorf("small file should succeed: %s", res.Content)
	}
}

// ---------- T-603: MaxWriteBytes ----------

func TestWriteFileMaxSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")

	tool := &WriteFile{MaxBytes: 10, CWD: dir}
	args, _ := json.Marshal(map[string]any{
		"path":    path,
		"content": strings.Repeat("hello", 10), // 50 bytes > 10 limit
	})
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error for oversized content")
	}
	if !strings.Contains(res.Content, "content too large") {
		t.Errorf("expected 'content too large', got: %s", res.Content)
	}
}

// ---------- T-604: path safety ----------

func TestPathSafetySymlinkEscape(t *testing.T) {
	dir := t.TempDir()

	// Create a symlink inside dir pointing outside.
	outsidePath := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outsidePath, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	symlinkPath := filepath.Join(dir, "escape_link")
	if err := os.Symlink(outsidePath, symlinkPath); err != nil {
		t.Fatal(err)
	}

	// read_file through symlink should fail because it escapes cwd.
	tool := &ReadFile{CWD: dir}
	args, _ := json.Marshal(map[string]any{"path": symlinkPath})
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error for symlink escape")
	}
	if !strings.Contains(res.Content, "escapes") {
		t.Errorf("expected 'escapes' in error, got: %s", res.Content)
	}
}

func TestPathSafetyNormalFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "normal.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := &ReadFile{CWD: dir}
	args, _ := json.Marshal(map[string]any{"path": path})
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Errorf("normal file should succeed: %s", res.Content)
	}
}

func TestPathSafetyNewFileWrite(t *testing.T) {
	dir := t.TempDir()

	// write_file creating a new file should succeed.
	tool := &WriteFile{CWD: dir}
	args, _ := json.Marshal(map[string]any{
		"path":    filepath.Join(dir, "new.txt"),
		"content": "hello",
	})
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Errorf("expected success: %s", res.Content)
	}
}

func TestPathSafetyNestedSymlink(t *testing.T) {
	dir := t.TempDir()

	// Create dir/subdir -> outside symlink chain.
	outsidePath := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outsidePath, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	subdir := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkPath := filepath.Join(subdir, "link")
	if err := os.Symlink(outsidePath, symlinkPath); err != nil {
		t.Fatal(err)
	}

	tool := &ReadFile{CWD: dir}
	args, _ := json.Marshal(map[string]any{"path": symlinkPath})
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error for nested symlink escape")
	}
}

// ---------- T-605: write_file refuses symlink ----------

func TestWriteFileRefuseSymlink(t *testing.T) {
	dir := t.TempDir()
	realPath := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(realPath, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	symlinkPath := filepath.Join(dir, "link.txt")
	if err := os.Symlink(realPath, symlinkPath); err != nil {
		t.Fatal(err)
	}

	tool := &WriteFile{CWD: dir}
	args, _ := json.Marshal(map[string]any{
		"path":    symlinkPath,
		"content": "should not write",
	})
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error for writing through symlink")
	}
	if !strings.Contains(res.Content, "symlink") {
		t.Errorf("expected symlink mention, got: %s", res.Content)
	}
}

// ---------- T-607: write_file size warning ----------

func TestWriteFileSizeWarning(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.txt")

	content := strings.Repeat("x", 1024*1024+1) // > 1 MB
	tool := &WriteFile{MaxBytes: 0, CWD: dir}   // default 5 MB limit
	args, _ := json.Marshal(map[string]any{
		"path":    path,
		"content": content,
	})
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Errorf("expected success with warning: %s", res.Content)
	}
	if !strings.Contains(res.Content, "warning: large write") {
		t.Errorf("expected size warning, got: %s", res.Content)
	}
}

// ---------- T-604: ResolveAndCheck returns canonical path ----------

func TestResolveAndCheckReturnsCanonical(t *testing.T) {
	dir := t.TempDir()
	// EvalSymlinks the dir itself so we know the canonical form.
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		realDir = dir
	}

	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, linkPath); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveAndCheck(linkPath, dir)
	if err != nil {
		t.Fatalf("ResolveAndCheck: %v", err)
	}
	// The canonical path must be the resolved form.
	want := filepath.Join(realDir, "target.txt")
	if got != want {
		t.Errorf("expected canonical path %q, got %q", want, got)
	}
}

func TestPathSafetyCwdIsSymlink(t *testing.T) {
	realCwd := t.TempDir()
	realCwdCanon, err := filepath.EvalSymlinks(realCwd)
	if err != nil {
		realCwdCanon = realCwd
	}

	linkCwd := filepath.Join(t.TempDir(), "cwd-link")
	if err := os.Symlink(realCwd, linkCwd); err != nil {
		t.Fatal(err)
	}

	// A file inside the real cwd should be accessible via the symlinked cwd.
	path := filepath.Join(realCwd, "ok.txt")
	if err := os.WriteFile(path, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveAndCheck(path, linkCwd)
	if err != nil {
		t.Fatalf("ResolveAndCheck with symlinked cwd: %v", err)
	}
	want := filepath.Join(realCwdCanon, "ok.txt")
	if got != want {
		t.Errorf("expected canonical %q, got %q", want, got)
	}
}

func TestIsWithin(t *testing.T) {
	tests := []struct {
		root, path string
		want       bool
	}{
		{"/a", "/a/b", true},
		{"/a", "/a", true},
		{"/a", "/a/b/c", true},
		{"/a", "/b", false},
		{"/a", "/a/../b", false},
		{"/a", "/ab", false},
	}
	for _, tc := range tests {
		got := isWithin(tc.root, tc.path)
		if got != tc.want {
			t.Errorf("isWithin(%q, %q) = %v, want %v", tc.root, tc.path, got, tc.want)
		}
	}
}

// ---------- T-604 edge: explicit outside-cwd paths rejected ----------

func TestPathSafetyExplicitOutsidePath(t *testing.T) {
	otherDir := filepath.Join(t.TempDir(), "other")
	if err := os.MkdirAll(otherDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outsidePath := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outsidePath, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := &ReadFile{CWD: otherDir}
	args, _ := json.Marshal(map[string]any{"path": outsidePath})
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error for path outside cwd")
	}
	if !strings.Contains(res.Content, "escapes") {
		t.Errorf("expected 'escapes' in error, got: %s", res.Content)
	}
}
