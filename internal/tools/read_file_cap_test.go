package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeNLines(t *testing.T, dir, name string, n int) {
	t.Helper()
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readCap(t *testing.T, dir string, args map[string]any) Result {
	t.Helper()
	raw, _ := json.Marshal(args)
	res, err := (&ReadFile{CWD: dir}).Execute(context.Background(), raw)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	return res
}

func TestReadFileWholeFileCapped(t *testing.T) {
	dir := t.TempDir()
	writeNLines(t, dir, "big.txt", 700)
	res := readCap(t, dir, map[string]any{"path": "big.txt"})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "\tline 500\n") {
		t.Errorf("expected line 500 present")
	}
	if strings.Contains(res.Content, "\tline 501\n") || strings.Contains(res.Content, "\tline 700\n") {
		t.Errorf("expected lines past cap (500) to be omitted")
	}
	if !strings.Contains(res.Content, "showing lines 1-500 of 700") {
		t.Errorf("expected truncation notice; tail=%q", res.Content[len(res.Content)-160:])
	}
}

func TestReadFileRangeNotCapped(t *testing.T) {
	dir := t.TempDir()
	writeNLines(t, dir, "big.txt", 700)
	res := readCap(t, dir, map[string]any{"path": "big.txt", "start_line": 600, "end_line": 660})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "\tline 600\n") || !strings.Contains(res.Content, "\tline 660\n") {
		t.Errorf("expected ranged lines 600-660 present")
	}
	if strings.Contains(res.Content, "truncated to keep") {
		t.Errorf("explicit range must not be capped")
	}
}

func TestReadFileSmallWholeUncapped(t *testing.T) {
	dir := t.TempDir()
	writeNLines(t, dir, "small.txt", 12)
	res := readCap(t, dir, map[string]any{"path": "small.txt"})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "\tline 12\n") {
		t.Errorf("expected full small file")
	}
	if strings.Contains(res.Content, "truncated to keep") {
		t.Errorf("small whole-file read must not be capped")
	}
}
