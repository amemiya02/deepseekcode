package tools

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateShortUnchanged(t *testing.T) {
	r := Result{Content: "hello", IsError: false}
	got := r.Truncate(100)
	if got.Content != "hello" || got.IsError != false {
		t.Errorf("short content modified: %+v", got)
	}
}

func TestTruncateLong(t *testing.T) {
	long := strings.Repeat("x", 1000)
	got := Result{Content: long}.Truncate(200)

	if !strings.Contains(got.Content, "truncated from 1000 bytes to 200 bytes") {
		t.Errorf("missing truncation footer: %q", got.Content[max(0, len(got.Content)-80):])
	}
	if len(got.Content) > 200 {
		t.Errorf("result len = %d, want <= 200", len(got.Content))
	}
}

func TestTruncatePreservesIsError(t *testing.T) {
	long := strings.Repeat("y", 1000)
	got := Result{Content: long, IsError: true}.Truncate(100)
	if !got.IsError {
		t.Error("IsError flag lost after truncation")
	}
}

func TestTruncateUTF8Safe(t *testing.T) {
	// Each Chinese char is 3 bytes in UTF-8.
	content := strings.Repeat("中", 500) // 1500 bytes
	got := Result{Content: content}.Truncate(200)

	if !utf8.ValidString(got.Content) {
		t.Errorf("truncation split a UTF-8 codepoint: %q", got.Content)
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
