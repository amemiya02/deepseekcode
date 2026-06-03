package codegraph_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/codegraph"
)

func TestIndexRebuild(t *testing.T) {
	dir := fixtureDir(t)
	idx := codegraph.NewIndex("github.com/amemiya02/deepseekcode/testdata/fixtures/simple")
	if err := idx.Rebuild(dir); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	results := idx.Search("Add")
	if len(results) == 0 {
		t.Fatal("Search(Add) returned no results after Rebuild")
	}
}

func TestIndexSearch(t *testing.T) {
	dir := fixtureDir(t)
	idx := codegraph.NewIndex("github.com/amemiya02/deepseekcode/testdata/fixtures/simple")
	_ = idx.Rebuild(dir)

	results := idx.Search("Multiply")
	if len(results) == 0 {
		t.Fatal("Search(Multiply) returned no results")
	}
	if results[0].Name != "Multiply" {
		t.Errorf("got name %q, want Multiply", results[0].Name)
	}
}

func TestIndexCallers(t *testing.T) {
	dir := fixtureDir(t)
	idx := codegraph.NewIndex("github.com/amemiya02/deepseekcode/testdata/fixtures/simple")
	_ = idx.Rebuild(dir)

	// Add is called by Run
	callers := idx.Callers("github.com/amemiya02/deepseekcode/testdata/fixtures/simple.Add")
	if len(callers) == 0 {
		t.Fatal("Callers(Add) returned no results; Run should call Add")
	}
	found := false
	for _, n := range callers {
		if n.Name == "Run" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected Run in callers of Add; got %v", callers)
	}
}

func TestIndexCallees(t *testing.T) {
	dir := fixtureDir(t)
	idx := codegraph.NewIndex("github.com/amemiya02/deepseekcode/testdata/fixtures/simple")
	_ = idx.Rebuild(dir)

	// Run calls Add and Multiply
	callees := idx.Callees("github.com/amemiya02/deepseekcode/testdata/fixtures/simple.Run")
	names := map[string]bool{}
	for _, n := range callees {
		names[n.Name] = true
	}
	if !names["Add"] || !names["Multiply"] {
		t.Errorf("expected Add and Multiply in callees of Run; got %v", names)
	}
}

func TestIndexImpact(t *testing.T) {
	dir := fixtureDir(t)
	idx := codegraph.NewIndex("github.com/amemiya02/deepseekcode/testdata/fixtures/simple")
	_ = idx.Rebuild(dir)

	// Changing Add impacts Run (Run calls Add)
	impacted := idx.Impact("github.com/amemiya02/deepseekcode/testdata/fixtures/simple.Add")
	names := map[string]bool{}
	for _, n := range impacted {
		names[n.Name] = true
	}
	if !names["Run"] {
		t.Errorf("expected Run in impact set of Add; got %v", names)
	}
}

func TestIndexIncremental(t *testing.T) {
	// Write a temp file, rebuild, modify it, rebuild again — only the changed
	// file should be re-parsed (we verify by observing that the new symbol appears).
	tmp := t.TempDir()
	f1 := filepath.Join(tmp, "a.go")
	os.WriteFile(f1, []byte("package tmp\nfunc Alpha() {}\n"), 0o644)

	idx := codegraph.NewIndex("tmp")
	if err := idx.Rebuild(tmp); err != nil {
		t.Fatalf("first Rebuild: %v", err)
	}
	if len(idx.Search("Alpha")) == 0 {
		t.Fatal("Alpha not found after first Rebuild")
	}

	// Add a new file
	f2 := filepath.Join(tmp, "b.go")
	os.WriteFile(f2, []byte("package tmp\nfunc Beta() {}\n"), 0o644)

	if err := idx.Rebuild(tmp); err != nil {
		t.Fatalf("second Rebuild: %v", err)
	}
	if len(idx.Search("Beta")) == 0 {
		t.Fatal("Beta not found after second Rebuild (incremental must parse new file)")
	}
	// Alpha must still be present
	if len(idx.Search("Alpha")) == 0 {
		t.Fatal("Alpha disappeared after second Rebuild")
	}
}
