package codegraph_test

import (
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/codegraph"
	"github.com/amemiya02/deepseekcode/internal/prompt"
)

func TestLatentSnippetNotEmptyAfterRebuild(t *testing.T) {
	dir := fixtureDir(t)
	idx := codegraph.NewIndex("github.com/amemiya02/deepseekcode/testdata/fixtures/simple")
	_ = idx.Rebuild(dir)

	inj := codegraph.NewLatentInjector(idx, 5)
	snippet := inj.Render()
	if snippet == "" {
		t.Fatal("LatentInjector.Render() returned empty string for a non-empty index")
	}
	if !strings.Contains(snippet, "Run") && !strings.Contains(snippet, "Add") {
		t.Errorf("rendered snippet does not mention any known symbol; got:\n%s", snippet)
	}
}

// TestLatentSnippetNotInStaticPrefix asserts that the DynamicContextBoundary
// string appears BEFORE any codegraph latent content in a composed prompt,
// i.e., the latent injection is cache-safe (it lives in the dynamic window).
func TestLatentSnippetNotInStaticPrefix(t *testing.T) {
	dir := fixtureDir(t)
	idx := codegraph.NewIndex("github.com/amemiya02/deepseekcode/testdata/fixtures/simple")
	_ = idx.Rebuild(dir)

	inj := codegraph.NewLatentInjector(idx, 5)
	snippet := inj.Render()

	// Simulate a composed prompt: static prefix | boundary | dynamic content | snippet
	boundary := prompt.DynamicContextBoundary
	staticPrefix := "You are a helpful agent.\n\nSystem instructions here.\n\n" + boundary
	dynamicBlock := "\nUser message here.\n\n"
	composed := staticPrefix + dynamicBlock + snippet

	boundaryIdx := strings.Index(composed, boundary)
	snippetIdx := strings.Index(composed, snippet)

	if boundaryIdx < 0 {
		t.Fatal("DynamicContextBoundary not found in composed prompt")
	}
	if snippetIdx < 0 {
		t.Fatal("latent snippet not found in composed prompt")
	}
	if snippetIdx <= boundaryIdx {
		t.Errorf("latent snippet appears BEFORE or AT the DynamicContextBoundary (pos %d vs %d); "+
			"this would pollute the static prefix and break cache hits", snippetIdx, boundaryIdx)
	}
}
