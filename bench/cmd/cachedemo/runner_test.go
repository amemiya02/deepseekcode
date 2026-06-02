// bench/cmd/cachedemo/runner_test.go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadUsageFixtureRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "u.json")
	if err := os.WriteFile(p, []byte(`[{"prompt_cache_hit_tokens":8000,"prompt_cache_miss_tokens":300,"completion_tokens":400}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := loadUsageFixture(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].PromptCacheHitTokens != 8000 {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}
