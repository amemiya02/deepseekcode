package codegraph_test

import (
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

func TestNodeID(t *testing.T) {
	n := codegraph.Node{
		ID:   "github.com/amemiya02/deepseekcode/internal/foo.Bar",
		Kind: codegraph.KindFunc,
		Name: "Bar",
		File: "internal/foo/bar.go",
		Line: 10,
	}
	if n.ID == "" {
		t.Fatal("Node.ID must not be empty")
	}
}
