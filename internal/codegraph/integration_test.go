//go:build integration

package codegraph_test

import (
	"os"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/codegraph"
)

func TestIntegrationRebuildRepoRoot(t *testing.T) {
	root := os.Getenv("REPO_ROOT")
	if root == "" {
		t.Skip("REPO_ROOT not set; skipping integration test")
	}
	idx := codegraph.NewIndex("github.com/amemiya02/deepseekcode")
	if err := idx.Rebuild(root); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	results := idx.Search("Search")
	if len(results) == 0 {
		t.Fatal("expected at least one node named Search in repo")
	}
	t.Logf("found %d node(s) for 'Search'", len(results))
}
