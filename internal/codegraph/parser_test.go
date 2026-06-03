package codegraph_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/codegraph"
)

func fixtureDir(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	// file is internal/codegraph/parser_test.go; go up two levels to repo root
	root := filepath.Join(filepath.Dir(file), "..", "..")
	return filepath.Join(root, "testdata", "fixtures", "simple")
}

func TestParserFindsNodes(t *testing.T) {
	dir := fixtureDir(t)
	store := codegraph.NewStore()
	if err := codegraph.ParseDir(dir, "github.com/amemiya02/deepseekcode/testdata/fixtures/simple", store); err != nil {
		t.Fatalf("ParseDir: %v", err)
	}
	wantFuncs := []string{"Add", "Multiply", "Run", "Calculator", "Doer"}
	found := map[string]bool{}
	for _, n := range store.AllNodes() {
		found[n.Name] = true
	}
	for _, name := range wantFuncs {
		if !found[name] {
			t.Errorf("expected node %q not found; got nodes: %v", name, nodeNames(store))
		}
	}
}

func TestParserFindsCallEdge(t *testing.T) {
	dir := fixtureDir(t)
	store := codegraph.NewStore()
	if err := codegraph.ParseDir(dir, "github.com/amemiya02/deepseekcode/testdata/fixtures/simple", store); err != nil {
		t.Fatalf("ParseDir: %v", err)
	}
	// Run() calls Add; there must be a CALLS edge from Run to Add
	runID := codegraph.NodeID("github.com/amemiya02/deepseekcode/testdata/fixtures/simple.Run")
	addID := codegraph.NodeID("github.com/amemiya02/deepseekcode/testdata/fixtures/simple.Add")
	for _, e := range store.OutEdges(runID) {
		if e.Kind == codegraph.EdgeCalls && e.To == addID {
			return
		}
	}
	t.Errorf("expected CALLS edge Run→Add; edges from Run: %v", store.OutEdges(runID))
}

func TestParserKinds(t *testing.T) {
	dir := fixtureDir(t)
	store := codegraph.NewStore()
	_ = codegraph.ParseDir(dir, "github.com/amemiya02/deepseekcode/testdata/fixtures/simple", store)

	calcID := codegraph.NodeID("github.com/amemiya02/deepseekcode/testdata/fixtures/simple.Calculator")
	doerID := codegraph.NodeID("github.com/amemiya02/deepseekcode/testdata/fixtures/simple.Doer")

	if n := store.Node(calcID); n == nil || n.Kind != codegraph.KindType {
		t.Errorf("Calculator should be KindType; got %+v", n)
	}
	if n := store.Node(doerID); n == nil || n.Kind != codegraph.KindInterface {
		t.Errorf("Doer should be KindInterface; got %+v", n)
	}
}

func nodeNames(s *codegraph.Store) []string {
	var out []string
	for _, n := range s.AllNodes() {
		out = append(out, n.Name)
	}
	return out
}
