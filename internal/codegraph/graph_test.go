package codegraph_test

import (
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/codegraph"
)

func TestNodeKindString(t *testing.T) {
	cases := []struct {
		k    codegraph.NodeKind
		want string
	}{
		{codegraph.KindFunc, "func"},
		{codegraph.KindType, "type"},
		{codegraph.KindInterface, "interface"},
	}
	for _, c := range cases {
		if got := c.k.String(); got != c.want {
			t.Errorf("NodeKind.String() = %q, want %q", got, c.want)
		}
	}
}

func TestEdgeKindString(t *testing.T) {
	cases := []struct {
		k    codegraph.EdgeKind
		want string
	}{
		{codegraph.EdgeCalls, "CALLS"},
		{codegraph.EdgeDefines, "DEFINES"},
		{codegraph.EdgeImplements, "IMPLEMENTS"},
		{codegraph.EdgeImports, "IMPORTS"},
	}
	for _, c := range cases {
		if got := c.k.String(); got != c.want {
			t.Errorf("EdgeKind.String() = %q, want %q", got, c.want)
		}
	}
}

// TestNodeIDFormat verifies that NodeID follows the "<pkg-path>.<Name>"
// convention by checking that a well-formed ID contains a dot separator.
func TestNodeIDFormat(t *testing.T) {
	id := codegraph.NodeID("github.com/amemiya02/deepseekcode/internal/foo.Bar")
	if !strings.Contains(string(id), ".") {
		t.Fatalf("NodeID %q does not contain a dot separator; expected format <pkg-path>.<Name>", id)
	}
	parts := strings.Split(string(id), ".")
	name := parts[len(parts)-1]
	if name == "" {
		t.Fatalf("NodeID %q has an empty symbol name after the last dot", id)
	}
}

// TestNewStore verifies that NewStore returns a store with all maps initialised
// (no nil-map panics on first use).
func TestNewStore(t *testing.T) {
	s := codegraph.NewStore()
	if s == nil {
		t.Fatal("NewStore() returned nil")
	}
	// AllNodes on an empty store must return an empty slice, not panic.
	if nodes := s.AllNodes(); len(nodes) != 0 {
		t.Fatalf("expected 0 nodes in fresh store, got %d", len(nodes))
	}
	// OutEdges / InEdges on unknown IDs must return nil, not panic.
	id := codegraph.NodeID("pkg.X")
	if edges := s.OutEdges(id); edges != nil {
		t.Fatalf("expected nil OutEdges for unknown ID, got %v", edges)
	}
	if edges := s.InEdges(id); edges != nil {
		t.Fatalf("expected nil InEdges for unknown ID, got %v", edges)
	}
}

// TestAddNode verifies insert and overwrite semantics.
func TestAddNode(t *testing.T) {
	s := codegraph.NewStore()
	id := codegraph.NodeID("pkg.Foo")

	n1 := &codegraph.Node{ID: id, Kind: codegraph.KindFunc, Name: "Foo", File: "pkg/foo.go", Line: 1}
	s.AddNode(n1)
	if got := s.Node(id); got != n1 {
		t.Fatalf("after AddNode, Node(%q) = %v, want %v", id, got, n1)
	}
	if len(s.AllNodes()) != 1 {
		t.Fatalf("expected 1 node, got %d", len(s.AllNodes()))
	}

	// Overwrite with a new pointer — must replace the old entry.
	n2 := &codegraph.Node{ID: id, Kind: codegraph.KindType, Name: "Foo", File: "pkg/foo.go", Line: 5}
	s.AddNode(n2)
	if got := s.Node(id); got != n2 {
		t.Fatalf("after overwrite AddNode, Node(%q) = %v, want %v", id, got, n2)
	}
	if len(s.AllNodes()) != 1 {
		t.Fatalf("expected 1 node after overwrite, got %d", len(s.AllNodes()))
	}
}

// TestAddEdgeDeduplication verifies that AddEdge inserts an edge once and
// keeps Out and In in sync, and that duplicate calls are silently ignored.
func TestAddEdgeDeduplication(t *testing.T) {
	s := codegraph.NewStore()
	from := codegraph.NodeID("pkg.Caller")
	to := codegraph.NodeID("pkg.Callee")
	e := codegraph.Edge{From: from, To: to, Kind: codegraph.EdgeCalls}

	// First insert.
	s.AddEdge(e)
	if got := s.OutEdges(from); len(got) != 1 {
		t.Fatalf("expected 1 out-edge after first AddEdge, got %d", len(got))
	}
	if got := s.InEdges(to); len(got) != 1 {
		t.Fatalf("expected 1 in-edge after first AddEdge, got %d", len(got))
	}

	// Duplicate insert — must be a no-op.
	s.AddEdge(e)
	if got := s.OutEdges(from); len(got) != 1 {
		t.Fatalf("expected 1 out-edge after duplicate AddEdge, got %d", len(got))
	}
	if got := s.InEdges(to); len(got) != 1 {
		t.Fatalf("expected 1 in-edge after duplicate AddEdge, got %d", len(got))
	}

	// A different edge kind should be treated as a distinct edge.
	e2 := codegraph.Edge{From: from, To: to, Kind: codegraph.EdgeImports}
	s.AddEdge(e2)
	if got := s.OutEdges(from); len(got) != 2 {
		t.Fatalf("expected 2 out-edges after adding a distinct edge kind, got %d", len(got))
	}
}
