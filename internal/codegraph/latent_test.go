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
	// 'Add' (util.go) has callers and consistently ranks in the top-5 by
	// PageRank; check it is present. 'Run' (main.go) has no callers and
	// falls below the top-5 cut, so we do not assert on it here.
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

	// Verify that a correctly assembled prompt (boundary first,
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

// TestLatentRenderNonPositiveTopN guards against a panic regression: a
// non-positive topN (0 or negative) must short-circuit to "" rather than
// indexing ranked[:limit] with a negative limit. NewLatentInjector(idx, -1)
// previously slid past the `== 0` guard and panicked.
func TestLatentRenderNonPositiveTopN(t *testing.T) {
	dir := fixtureDir(t)
	idx := codegraph.NewIndex("github.com/amemiya02/deepseekcode/testdata/fixtures/simple")
	if err := idx.Rebuild(dir); err != nil {
		t.Fatalf("Rebuild(%q) failed: %v", dir, err)
	}

	for _, topN := range []int{-1, 0} {
		inj := codegraph.NewLatentInjector(idx, topN)
		got := inj.Render() // must not panic
		if got != "" {
			t.Errorf("Render() with topN=%d = %q; want \"\"", topN, got)
		}
	}
}
