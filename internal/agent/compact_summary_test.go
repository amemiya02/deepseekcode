package agent

import (
	"strings"
	"testing"
)

func TestCollectKeyFilesExtractsCommonExtensions(t *testing.T) {
	into := map[string]struct{}{}
	collectKeyFiles("touch parser.go and pkg/util.rs; also docs/spec.md", into)
	got := sortedKeys(into)
	want := []string{"docs/spec.md", "parser.go", "pkg/util.rs"}
	if !slicesEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestCollectKeyFilesDedups(t *testing.T) {
	into := map[string]struct{}{}
	collectKeyFiles("parser.go parser.go parser.go", into)
	if len(into) != 1 {
		t.Errorf("expected 1 unique entry, got %d (%v)", len(into), sortedKeys(into))
	}
}

func TestCollectKeyFilesIgnoresUnknownExtensions(t *testing.T) {
	into := map[string]struct{}{}
	collectKeyFiles("touch parser.xyz and image.png", into)
	if len(into) != 0 {
		t.Errorf("expected 0 entries for unknown extensions, got %v", sortedKeys(into))
	}
}

func TestCollectPendingKeepsSentencesWithSignalWords(t *testing.T) {
	into := map[string]struct{}{}
	collectPending("First sentence. TODO: refactor parser. Done!", into)
	if len(into) != 1 {
		t.Fatalf("expected 1 pending sentence; got %v", sortedKeys(into))
	}
	for k := range into {
		if !strings.Contains(strings.ToLower(k), "todo") {
			t.Errorf("captured sentence %q missing signal word", k)
		}
	}
}

func TestCollectPendingHandlesMultipleSignalWords(t *testing.T) {
	into := map[string]struct{}{}
	collectPending("Need to follow up on auth. Pending review of db migration.", into)
	if len(into) != 2 {
		t.Errorf("expected 2 sentences (follow up + pending); got %d (%v)", len(into), sortedKeys(into))
	}
}

func TestCollectPendingDedupsAcrossCalls(t *testing.T) {
	into := map[string]struct{}{}
	collectPending("TODO: ship", into)
	collectPending("TODO: ship", into)
	if len(into) != 1 {
		t.Errorf("expected dedup; got %d", len(into))
	}
}

func TestTruncateRunesRespectsRuneBoundary(t *testing.T) {
	in := "你好世界"
	got := truncateRunes(in, 2)
	if got != "你好…" {
		t.Errorf("expected '你好…', got %q", got)
	}
}

func TestSingleLineCollapsesWhitespace(t *testing.T) {
	got := singleLine("a   b\tc\n\nd")
	if got != "a b c d" {
		t.Errorf("got %q", got)
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
