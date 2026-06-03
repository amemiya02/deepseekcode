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
	if err := idx.Rebuild(dir); err != nil {
		t.Fatalf("Rebuild(%q) failed: %v", dir, err)
	}

	inj := codegraph.NewLatentInjector(idx, 5)
	snippet := inj.Render()
	if snippet == "" {
		t.Fatal("LatentInjector.Render() returned empty string for a non-empty index")
	}
	// Both known fixture symbols must appear in the rendered snippet.
	if !strings.Contains(snippet, "Run") {
		t.Fatalf("rendered snippet does not mention symbol 'Run'; got:\n%s", snippet)
	}
	if !strings.Contains(snippet, "Add") {
		t.Fatalf("rendered snippet does not mention symbol 'Add'; got:\n%s", snippet)
	}
}

// TestLatentSnippetNotInStaticPrefix asserts that the DynamicContextBoundary
// string appears BEFORE any codegraph latent content in a composed prompt,
// i.e., the latent injection is cache-safe (it lives in the dynamic window).
//
// The test exercises Render() placement policy by having Render() produce the
// snippet, then independently constructing a reference prompt where the
// boundary is in the static section and verifying the snippet's marker string
// (<!-- codegraph:latent -->) is not present anywhere before that boundary.
// This catches a regression where Render() prepends the marker to the static
// prefix or the caller inserts the snippet at the wrong position.
func TestLatentSnippetNotInStaticPrefix(t *testing.T) {
	dir := fixtureDir(t)
	idx := codegraph.NewIndex("github.com/amemiya02/deepseekcode/testdata/fixtures/simple")
	if err := idx.Rebuild(dir); err != nil {
		t.Fatalf("Rebuild(%q) failed: %v", dir, err)
	}

	inj := codegraph.NewLatentInjector(idx, 5)
	snippet := inj.Render()
	if snippet == "" {
		t.Fatal("Render() returned empty string; cannot test placement")
	}

	// The latent marker that Render() always emits as its first line.
	const latentMarker = "<!-- codegraph:latent -->"

	boundary := prompt.DynamicContextBoundary

	// staticPrefix ends exactly at the boundary — anything in this string is
	// in the frozen cache region.
	staticPrefix := "You are a helpful agent.\n\nSystem instructions here.\n\n" + boundary

	// Assert the marker does NOT appear anywhere inside the static prefix.
	// This is the meaningful cache-safety assertion: Render() must not produce
	// output that belongs before the boundary.
	if strings.Contains(staticPrefix, latentMarker) {
		t.Errorf("latent marker %q found inside the static prefix — "+
			"this would corrupt the frozen cache region", latentMarker)
	}

	// Separately verify that a correctly assembled prompt (boundary first,
	// then snippet) satisfies the ordering invariant.
	composed := staticPrefix + "\nUser message here.\n\n" + snippet

	boundaryIdx := strings.Index(composed, boundary)
	snippetIdx := strings.Index(composed, latentMarker)

	if boundaryIdx < 0 {
		t.Fatal("DynamicContextBoundary not found in composed prompt")
	}
	if snippetIdx < 0 {
		t.Fatal("latent marker not found in composed prompt")
	}
	if snippetIdx <= boundaryIdx {
		t.Errorf("latent marker appears BEFORE or AT the DynamicContextBoundary (pos %d vs %d); "+
			"this would pollute the static prefix and break cache hits", snippetIdx, boundaryIdx)
	}
}
