package codegraph_test

import (
	"os"
	"path/filepath"
	"sync"
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
	if err := idx.Rebuild(dir); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

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
	if err := idx.Rebuild(dir); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

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
	if err := idx.Rebuild(dir); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

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
	if err := idx.Rebuild(dir); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

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

func TestIndexDeletion(t *testing.T) {
	// Write a file containing a symbol, rebuild, delete the file, rebuild again,
	// and assert the symbol is absent. Guards against regression of the silent
	// deletion bug fixed in commit 605002f (dead nodes not removed on file delete).
	tmp := t.TempDir()
	f1 := filepath.Join(tmp, "del.go")
	if err := os.WriteFile(f1, []byte("package tmp\nfunc ToDelete() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile del.go: %v", err)
	}

	idx := codegraph.NewIndex("tmp")
	if err := idx.Rebuild(tmp); err != nil {
		t.Fatalf("first Rebuild: %v", err)
	}
	if len(idx.Search("ToDelete")) == 0 {
		t.Fatal("ToDelete not found after first Rebuild")
	}

	// Delete the file and rebuild.
	if err := os.Remove(f1); err != nil {
		t.Fatalf("Remove del.go: %v", err)
	}
	if err := idx.Rebuild(tmp); err != nil {
		t.Fatalf("second Rebuild after deletion: %v", err)
	}
	if len(idx.Search("ToDelete")) != 0 {
		t.Fatal("ToDelete still present after its source file was deleted")
	}
}

func TestIndexIncremental(t *testing.T) {
	// Write a temp file, rebuild, add a second file, rebuild again — verify that
	// symbols from both files are visible and no symbol is lost across rebuilds.
	// Note: the implementation performs a full re-parse on every rebuild; the
	// observable contract is that newly added symbols appear and pre-existing
	// symbols are retained, not that only changed files are re-parsed.
	tmp := t.TempDir()
	f1 := filepath.Join(tmp, "a.go")
	if err := os.WriteFile(f1, []byte("package tmp\nfunc Alpha() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile a.go: %v", err)
	}

	idx := codegraph.NewIndex("tmp")
	if err := idx.Rebuild(tmp); err != nil {
		t.Fatalf("first Rebuild: %v", err)
	}
	if len(idx.Search("Alpha")) == 0 {
		t.Fatal("Alpha not found after first Rebuild")
	}

	// Add a new file
	f2 := filepath.Join(tmp, "b.go")
	if err := os.WriteFile(f2, []byte("package tmp\nfunc Beta() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile b.go: %v", err)
	}

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

func TestIndexLookup(t *testing.T) {
	dir := fixtureDir(t)
	idx := codegraph.NewIndex("github.com/amemiya02/deepseekcode/testdata/fixtures/simple")
	if err := idx.Rebuild(dir); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	// Add is a known symbol in the fixture (see TestIndexCallers).
	const knownID = "github.com/amemiya02/deepseekcode/testdata/fixtures/simple.Add"
	n := idx.Lookup(knownID)
	if n == nil {
		t.Fatalf("Lookup(%q) returned nil for a known symbol", knownID)
	}
	if n.Name != "Add" {
		t.Errorf("Lookup(%q).Name = %q, want Add", knownID, n.Name)
	}

	// An ID that does not exist must return nil, not a stale or zero node.
	if got := idx.Lookup("github.com/amemiya02/deepseekcode/testdata/fixtures/simple.DoesNotExist"); got != nil {
		t.Errorf("Lookup of unknown ID returned non-nil node: %+v", got)
	}
}

// TestIndexLookupConcurrentRebuild guards the Lookup/Rebuild data race: Lookup
// takes idx.mu.RLock around both the store dereference and the map lookup,
// while Rebuild atomically swaps idx.store under the write lock. Run with -race.
func TestIndexLookupConcurrentRebuild(t *testing.T) {
	dir := fixtureDir(t)
	idx := codegraph.NewIndex("github.com/amemiya02/deepseekcode/testdata/fixtures/simple")
	if err := idx.Rebuild(dir); err != nil {
		t.Fatalf("initial Rebuild: %v", err)
	}

	const knownID = "github.com/amemiya02/deepseekcode/testdata/fixtures/simple.Add"

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			if err := idx.Rebuild(dir); err != nil {
				t.Errorf("Rebuild in loop: %v", err)
				return
			}
		}
	}()

	// Hammer Lookup concurrently with the rebuild loop. We assert nothing about
	// the result here (a rebuild may be mid-flight); the point is the race
	// detector, not the value.
	for i := 0; i < 1000; i++ {
		_ = idx.Lookup(knownID)
	}

	wg.Wait()
}
