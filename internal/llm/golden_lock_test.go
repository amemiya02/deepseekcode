// internal/llm/golden_lock_test.go
package llm

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// goldenRequest is the fixed input whose serialised form must never drift.
// Edit this constant only when the wire format intentionally changes,
// and re-commit the golden file at the same time.
func goldenRequest() Request {
	return Request{
		Model: "deepseek-chat",
		Messages: []Message{
			{
				Role:   "user",
				Blocks: []ContentBlock{TextBlock{Text: "What is 2+2?"}},
			},
			{
				Role:   "assistant",
				Blocks: []ContentBlock{TextBlock{Text: "4"}},
			},
			{
				Role:   "user",
				Blocks: []ContentBlock{TextBlock{Text: "Are you sure?"}},
			},
		},
		Stream: true,
	}
}

const goldenDir = "golden"
const goldenFile = "marshal_cache_stable.golden"

func TestMarshalCacheStable_GoldenLock(t *testing.T) {
	t.Setenv("DEEPSEEKCODE_REASONING_DROP", "")
	t.Setenv("DEEPSEEKCODE_REASONING_RETAIN", "")

	req := goldenRequest()
	got, err := req.MarshalCacheStable()
	if err != nil {
		t.Fatalf("MarshalCacheStable: %v", err)
	}

	goldenPath := filepath.Join(goldenDir, goldenFile)

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(goldenDir, 0o755); err != nil {
			t.Fatalf("mkdir golden: %v", err)
		}
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("golden file updated: %s", goldenPath)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden file %s: %v\n\nHint: run with UPDATE_GOLDEN=1 to generate it.", goldenPath, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("MarshalCacheStable output differs from golden.\n"+
			"Got  (%d bytes): %s\n"+
			"Want (%d bytes): %s\n\n"+
			"If this change is intentional, re-run with UPDATE_GOLDEN=1 and commit the new golden file.",
			len(got), got, len(want), want)
	}
}
