package structsearch

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSearchGoFunctions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.go")
	if err := os.WriteFile(path, []byte("package sample\n\nfunc Alpha() {}\nfunc beta() {}\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	results, err := Search(dir, Query{Language: "go", Kind: "function", Name: "Alpha"})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) != 1 || results[0].Name != "Alpha" {
		t.Fatalf("results = %+v, want Alpha", results)
	}
}
