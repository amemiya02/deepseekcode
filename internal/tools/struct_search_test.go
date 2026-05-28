package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStructSearchToolExecute(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sample.go"), []byte("package sample\n\nfunc Alpha() {}\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	tool := NewStructSearchTool(dir)
	res, err := tool.Execute(context.Background(), []byte(`{"kind":"function","name":"Alpha"}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.IsError {
		t.Fatalf("Execute returned tool error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "sample.go:3 Alpha") {
		t.Fatalf("unexpected content:\n%s", res.Content)
	}
}
