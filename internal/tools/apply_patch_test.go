package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func testApplyPatchTool(t *testing.T) *ApplyPatchTool {
	t.Helper()
	cwd := t.TempDir()
	return NewApplyPatchTool(cwd, 5*1024*1024)
}

func TestApplyPatchAddNewFile(t *testing.T) {
	tool := testApplyPatchTool(t)
	cwd := tool.cwd
	res, err := tool.Execute(context.Background(), mustPatchArgs(`*** Begin Patch
*** Add File: hello.txt
+hello world
*** End Patch`))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	data, err := os.ReadFile(filepath.Join(cwd, "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello world" {
		t.Errorf("got %q, want %q", string(data), "hello world")
	}
}

func TestApplyPatchUpdateFile(t *testing.T) {
	tool := testApplyPatchTool(t)
	cwd := tool.cwd
	orig := filepath.Join(cwd, "f.go")
	if err := os.WriteFile(orig, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := tool.Execute(context.Background(), mustPatchArgs(`*** Begin Patch
*** Update File: f.go
@@
 a
-b
+B
 c
*** End Patch`))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	data, err := os.ReadFile(orig)
	if err != nil {
		t.Fatal(err)
	}
	want := "a\nB\nc\n"
	if string(data) != want {
		t.Errorf("got:\n%s\nwant:\n%s", string(data), want)
	}
}

func TestApplyPatchDeleteFile(t *testing.T) {
	tool := testApplyPatchTool(t)
	cwd := tool.cwd
	f := filepath.Join(cwd, "old.go")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := tool.Execute(context.Background(), mustPatchArgs(`*** Begin Patch
*** Delete File: old.go
*** End Patch`))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Error("file should be deleted")
	}
}

func TestApplyPatchMoveFile(t *testing.T) {
	tool := testApplyPatchTool(t)
	cwd := tool.cwd
	old := filepath.Join(cwd, "old.go")
	if err := os.WriteFile(old, []byte("line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := tool.Execute(context.Background(), mustPatchArgs(`*** Begin Patch
*** Update File: old.go
*** Move to: new.go
@@
 line
*** End Patch`))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("old file should be removed after move")
	}
	data, err := os.ReadFile(filepath.Join(cwd, "new.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "line\n" {
		t.Errorf("got %q, want %q", string(data), "line\n")
	}
}

func TestApplyPatchPathEscape(t *testing.T) {
	tool := testApplyPatchTool(t)
	res, err := tool.Execute(context.Background(), mustPatchArgs(`*** Begin Patch
*** Add File: ../etc/evil
+bad
*** End Patch`))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error for path escape")
	}
}

func TestApplyPatchAddExistingError(t *testing.T) {
	tool := testApplyPatchTool(t)
	cwd := tool.cwd
	if err := os.WriteFile(filepath.Join(cwd, "exists.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := tool.Execute(context.Background(), mustPatchArgs(`*** Begin Patch
*** Add File: exists.txt
+new
*** End Patch`))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error for adding existing file")
	}
}

func TestApplyPatchUpdateMissingError(t *testing.T) {
	tool := testApplyPatchTool(t)
	res, err := tool.Execute(context.Background(), mustPatchArgs(`*** Begin Patch
*** Update File: nope.go
@@
-x
+y
*** End Patch`))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error for updating missing file")
	}
}

func TestApplyPatchOversizedWrite(t *testing.T) {
	cwd := t.TempDir()
	tool := NewApplyPatchTool(cwd, 10) // tiny limit
	res, err := tool.Execute(context.Background(), mustPatchArgs(`*** Begin Patch
*** Add File: big.txt
+this is way more than ten bytes
*** End Patch`))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error for oversized write")
	}
}

func TestApplyPatchAffectedPaths(t *testing.T) {
	tool := testApplyPatchTool(t)
	args := mustPatchArgs(`*** Begin Patch
*** Add File: a.txt
+hi
*** Update File: b.go
@@
-old
+new
*** Delete File: c.go
*** End Patch`)
	paths := tool.AffectedPaths(args)
	if len(paths) != 3 {
		t.Fatalf("got %d paths, want 3: %v", len(paths), paths)
	}
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			t.Errorf("path %q should be absolute", p)
		}
	}
}

func TestApplyPatchAffectedPathsWithMove(t *testing.T) {
	tool := testApplyPatchTool(t)
	args := mustPatchArgs(`*** Begin Patch
*** Update File: old.go
*** Move to: new.go
@@
 line
*** End Patch`)
	paths := tool.AffectedPaths(args)
	if len(paths) != 2 {
		t.Fatalf("got %d paths, want 2 (path + movePath): %v", len(paths), paths)
	}
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			t.Errorf("path %q should be absolute", p)
		}
	}
}

func TestApplyPatchEmptyPatchText(t *testing.T) {
	tool := testApplyPatchTool(t)
	res, err := tool.Execute(context.Background(), mustPatchArgs(""))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error for empty patchText")
	}
}

func TestApplyPatchMultiFileMixed(t *testing.T) {
	tool := testApplyPatchTool(t)
	cwd := tool.cwd
	// Pre-create update target
	if err := os.WriteFile(filepath.Join(cwd, "b.go"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := tool.Execute(context.Background(), mustPatchArgs(`*** Begin Patch
*** Add File: a.txt
+hello
*** Update File: b.go
@@
-old
+new
*** Delete File: c.go
*** End Patch`))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	// Verify a.txt created
	data, _ := os.ReadFile(filepath.Join(cwd, "a.txt"))
	if string(data) != "hello" {
		t.Errorf("a.txt: got %q", string(data))
	}
	// Verify b.go updated
	data, _ = os.ReadFile(filepath.Join(cwd, "b.go"))
	if string(data) != "new\n" {
		t.Errorf("b.go: got %q", string(data))
	}
}

func TestApplyPatchEndOfFileAnchor(t *testing.T) {
	tool := testApplyPatchTool(t)
	cwd := tool.cwd
	// Pre-create file to update with EOF append
	if err := os.WriteFile(filepath.Join(cwd, "f.go"), []byte("a\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := tool.Execute(context.Background(), mustPatchArgs(`*** Begin Patch
*** Update File: f.go
@@
 a
 b
+c
*** End of File
*** End Patch`))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	data, _ := os.ReadFile(filepath.Join(cwd, "f.go"))
	want := "a\nb\nc\n"
	if string(data) != want {
		t.Errorf("got:\n%s\nwant:\n%s", string(data), want)
	}
}

func TestApplyPatchEscapeBlocksAllWrites(t *testing.T) {
	tool := testApplyPatchTool(t)
	cwd := tool.cwd
	// Valid Add followed by path-escape Add: entire patch must be rejected
	// and the first file must NOT exist on disk.
	res, err := tool.Execute(context.Background(), mustPatchArgs(`*** Begin Patch
*** Add File: good.txt
+hello
*** Add File: ../escape.txt
+evil
*** End Patch`))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error for path escape in second hunk")
	}
	if _, err := os.Stat(filepath.Join(cwd, "good.txt")); !os.IsNotExist(err) {
		t.Error("good.txt should NOT exist — path escape must block all writes")
	}
}

func mustPatchArgs(patchText string) []byte {
	return []byte(`{"patchText":"` + escapeJSON(patchText) + `"}`)
}

func escapeJSON(s string) string {
	var out []byte
	for _, b := range []byte(s) {
		switch b {
		case '"':
			out = append(out, '\\', '"')
		case '\\':
			out = append(out, '\\', '\\')
		case '\n':
			out = append(out, '\\', 'n')
		case '\r':
			out = append(out, '\\', 'r')
		case '\t':
			out = append(out, '\\', 't')
		default:
			out = append(out, b)
		}
	}
	return string(out)
}
