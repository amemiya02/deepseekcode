package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTree materializes a set of files (relative paths under root, slash- or
// OS-separated) creating parent dirs as needed, so the walk tests have a real
// tree to scan.
func writeTree(t *testing.T, root string, rels ...string) {
	t.Helper()
	for _, rel := range rels {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", p, err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
}

// TestWalkFilesSkipsNoiseAndDotDirs verifies the walk returns ordinary files as
// slash-separated repo-relative paths and prunes .git, node_modules, vendor,
// and any dot-directory wholesale.
func TestWalkFilesSkipsNoiseAndDotDirs(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root,
		"main.go",
		"pkg/util/helper.go",
		".git/config",
		".git/objects/ab/cd",
		"node_modules/left-pad/index.js",
		"vendor/dep/dep.go",
		".idea/workspace.xml",
		".deepseek/skills/foo/SKILL.md",
	)

	got := walkFiles(root, fileIndexCap)
	want := map[string]bool{
		"main.go":            true,
		"pkg/util/helper.go": true,
	}
	if len(got) != len(want) {
		t.Fatalf("walkFiles returned %v, want exactly %d entries", got, len(want))
	}
	for _, p := range got {
		if !want[p] {
			t.Fatalf("walkFiles returned unexpected path %q (full set %v)", p, got)
		}
		if strings.ContainsRune(p, os.PathSeparator) && os.PathSeparator != '/' {
			t.Fatalf("path %q is not slash-normalized", p)
		}
	}
	// Result must be sorted for a deterministic candidate order.
	for i := 1; i < len(got); i++ {
		if got[i-1] > got[i] {
			t.Fatalf("walkFiles result not sorted: %v", got)
		}
	}
}

// TestWalkFilesRespectsCap verifies the cap bounds the candidate set even on a
// wide tree.
func TestWalkFilesRespectsCap(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 50; i++ {
		writeTree(t, root, filepath.Join("dir", "f"+itoa2(i)+".txt"))
	}
	got := walkFiles(root, 10)
	if len(got) != 10 {
		t.Fatalf("walkFiles with cap 10 returned %d entries, want 10", len(got))
	}
}

// TestWalkFilesDepthLimit verifies the walk stops descending past the max depth
// so a pathologically deep tree can't blow up the index.
func TestWalkFilesDepthLimit(t *testing.T) {
	root := t.TempDir()
	// A file at depth 2 (kept) and one buried well past the limit (pruned).
	deep := make([]string, fileIndexMaxDepth+3)
	for i := range deep {
		deep[i] = "d"
	}
	writeTree(t, root,
		"a/shallow.go",
		strings.Join(deep, "/")+"/buried.go",
	)
	got := walkFiles(root, fileIndexCap)
	for _, p := range got {
		if strings.Contains(p, "buried.go") {
			t.Fatalf("walkFiles should have pruned the over-deep file, got %v", got)
		}
	}
	var sawShallow bool
	for _, p := range got {
		if p == "a/shallow.go" {
			sawShallow = true
		}
	}
	if !sawShallow {
		t.Fatalf("walkFiles dropped the shallow file: %v", got)
	}
}

// TestWalkFilesBadRoot verifies a blank or non-existent root yields nil rather
// than walking the process cwd or panicking.
func TestWalkFilesBadRoot(t *testing.T) {
	if got := walkFiles("", fileIndexCap); got != nil {
		t.Fatalf("walkFiles(\"\") = %v, want nil", got)
	}
	if got := walkFiles(filepath.Join(t.TempDir(), "does-not-exist"), fileIndexCap); len(got) != 0 {
		t.Fatalf("walkFiles(missing) = %v, want empty", got)
	}
}

// TestFileCompletionItemsRoundTrip verifies paths map to rows whose insert and
// label are the path itself and whose kind is fileCmd.
func TestFileCompletionItemsRoundTrip(t *testing.T) {
	items := fileCompletionItems([]string{"a.go", "pkg/b.go"})
	if len(items) != 2 {
		t.Fatalf("fileCompletionItems len = %d, want 2", len(items))
	}
	for _, it := range items {
		if it.insert != it.label {
			t.Fatalf("file row insert %q != label %q", it.insert, it.label)
		}
		if it.kind != fileCmd {
			t.Fatalf("file row kind = %v, want fileCmd", it.kind)
		}
	}
}
