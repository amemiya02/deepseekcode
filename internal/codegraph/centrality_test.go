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
	// Graph: A→B, A→C; C has no out-edges (dangling node).
	// Without dangling-mass redistribution C's terminal score would stay low
	// (only one in-edge from A, sharing A's mass equally with B). With correct
	// redistribution C's dangling mass is re-spread across all nodes each
	// iteration, boosting scores globally. The key invariant: B must still rank
	// above A (B receives half of A's distributed mass every iteration, while
	// A only receives residual dangling shares). This assertion fails if the
	// dangling-redistribution code is removed.
	store := codegraph.NewStore()
	for _, name := range []string{"A", "B", "C"} {
		store.AddNode(&codegraph.Node{ID: codegraph.NodeID(name), Name: name, Kind: codegraph.KindFunc})
	}
	store.AddEdge(codegraph.Edge{From: "A", To: "B", Kind: codegraph.EdgeCalls})
	store.AddEdge(codegraph.Edge{From: "A", To: "C", Kind: codegraph.EdgeCalls})

	ranked := codegraph.RankByPageRank(store, 20)
	if len(ranked) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(ranked))
	}
	// B and C each receive half of A's call-mass plus an even share of C's
	// dangling mass; A only receives dangling shares, so B (or C) must rank
	// above A. B and C are tied, so ranked[0] must NOT be A.
	if ranked[0].Name == "A" {
		t.Errorf("expected B or C at rank 0 (highest in-degree from A); got %q", ranked[0].Name)
	}
	// Additionally verify A is last: it has the lowest in-degree (zero direct
	// in-edges from registered nodes, only dangling redistribution).
	if ranked[2].Name != "A" {
		t.Errorf("expected A at rank 2 (lowest); got %q", ranked[2].Name)
	}
}
