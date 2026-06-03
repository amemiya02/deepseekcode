package codegraph_test

import (
	"testing"

	"github.com/amemiya02/deepseekcode/internal/codegraph"
)

func TestCentralityRankOrder(t *testing.T) {
	// Build a tiny store: A calls B, C calls B, D calls B.
	// B has in-degree 3 and should rank highest.
	store := codegraph.NewStore()
	for _, name := range []string{"A", "B", "C", "D"} {
		store.AddNode(&codegraph.Node{ID: codegraph.NodeID(name), Name: name, Kind: codegraph.KindFunc})
	}
	store.AddEdge(codegraph.Edge{From: "A", To: "B", Kind: codegraph.EdgeCalls})
	store.AddEdge(codegraph.Edge{From: "C", To: "B", Kind: codegraph.EdgeCalls})
	store.AddEdge(codegraph.Edge{From: "D", To: "B", Kind: codegraph.EdgeCalls})

	ranked := codegraph.RankByPageRank(store, 20)
	if len(ranked) == 0 {
		t.Fatal("RankByPageRank returned empty slice")
	}
	if ranked[0].Name != "B" {
		t.Errorf("expected B at rank 0 (highest in-degree); got %q", ranked[0].Name)
	}
}

func TestCentralityEmptyStore(t *testing.T) {
	store := codegraph.NewStore()
	ranked := codegraph.RankByPageRank(store, 20)
	if len(ranked) != 0 {
		t.Fatalf("RankByPageRank(empty) must return empty slice; got %d elements", len(ranked))
	}
}

func TestCentralityDanglingNode(t *testing.T) {
	// A calls B; C has no out-edges (dangling node).
	// After PageRank C's score should be redistributed evenly,
	// so B (high in-degree from A) should still rank above A.
	store := codegraph.NewStore()
	for _, name := range []string{"A", "B", "C"} {
		store.AddNode(&codegraph.Node{ID: codegraph.NodeID(name), Name: name, Kind: codegraph.KindFunc})
	}
	store.AddEdge(codegraph.Edge{From: "A", To: "B", Kind: codegraph.EdgeCalls})

	ranked := codegraph.RankByPageRank(store, 20)
	if len(ranked) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(ranked))
	}
	// B receives all of A's call-mass plus an even share of C's dangling mass,
	// so B must rank first.
	if ranked[0].Name != "B" {
		t.Errorf("expected B at rank 0; got %q", ranked[0].Name)
	}
}
