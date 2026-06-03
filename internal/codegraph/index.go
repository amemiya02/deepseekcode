package codegraph

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Index is the primary entry point for the codegraph. It manages incremental
// parsing and exposes query methods.
type Index struct {
	pkgPath string
	mu      sync.RWMutex
	store   *Store
	hashes  map[string]string // filepath → sha256 hex
}

// NewIndex returns an empty Index for the given module-qualified package path.
func NewIndex(pkgPath string) *Index {
	return &Index{
		pkgPath: pkgPath,
		store:   NewStore(),
		hashes:  make(map[string]string),
	}
}

// Rebuild walks root, re-parses files whose content hash changed, and removes
// nodes/edges for files that disappeared. It is safe to call repeatedly.
func (idx *Index) Rebuild(root string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	seen := map[string]bool{}
	var changedFiles []string
	// pendingHashes accumulates new hashes; only committed after a successful parse.
	pendingHashes := map[string]string{}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		seen[path] = true
		h, hErr := hashFile(path)
		if hErr != nil {
			return nil // skip unreadable files
		}
		if idx.hashes[path] != h {
			changedFiles = append(changedFiles, path)
			pendingHashes[path] = h
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk %s: %w", root, err)
	}

	// Identify deleted files (present in hashes but not in the current walk).
	var deletedPaths []string
	for path := range idx.hashes {
		if !seen[path] {
			deletedPaths = append(deletedPaths, path)
		}
	}

	// Re-parse changed files by re-building the whole store.
	// (Full re-parse is correct; file-level incremental is a later optimisation.)
	if len(changedFiles) > 0 || len(deletedPaths) > 0 || len(idx.store.AllNodes()) == 0 {
		fresh := NewStore()
		if pErr := parseAll(root, idx.pkgPath, fresh); pErr != nil {
			return pErr
		}
		idx.store = fresh

		// Only commit hash updates and deletions after a successful parse.
		for path, h := range pendingHashes {
			idx.hashes[path] = h
		}
		for _, path := range deletedPaths {
			delete(idx.hashes, path)
		}
	}
	return nil
}

// Search returns all nodes whose Name matches the query (exact match first,
// then prefix matches when no exact match is found).
func (idx *Index) Search(name string) []*Node {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var exact, prefix []*Node
	for _, n := range idx.store.AllNodes() {
		if n.Name == name {
			exact = append(exact, n)
		} else if strings.HasPrefix(n.Name, name) {
			prefix = append(prefix, n)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	return prefix
}

// Callers returns all nodes that have a CALLS edge pointing to symID.
func (idx *Index) Callers(symID NodeID) []*Node {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var out []*Node
	for _, e := range idx.store.InEdges(symID) {
		if e.Kind == EdgeCalls {
			if n := idx.store.Node(e.From); n != nil {
				out = append(out, n)
			}
		}
	}
	return out
}

// Callees returns all nodes that symID has a CALLS edge to.
func (idx *Index) Callees(symID NodeID) []*Node {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var out []*Node
	for _, e := range idx.store.OutEdges(symID) {
		if e.Kind == EdgeCalls {
			if n := idx.store.Node(e.To); n != nil {
				out = append(out, n)
			}
		}
	}
	return out
}

// Impact performs a reverse-BFS from symID over CALLS edges and returns all
// nodes that transitively call symID (i.e., would be affected by a change).
func (idx *Index) Impact(symID NodeID) []*Node {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	visited := map[NodeID]bool{symID: true}
	queue := []NodeID{symID}
	var out []*Node
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, e := range idx.store.InEdges(cur) {
			if e.Kind != EdgeCalls {
				continue
			}
			if visited[e.From] {
				continue
			}
			visited[e.From] = true
			if n := idx.store.Node(e.From); n != nil {
				out = append(out, n)
				queue = append(queue, e.From)
			}
		}
	}
	return out
}

// Lookup returns the Node with the given id, or nil if not present.
// It holds idx.mu.RLock for both the store dereference and the map lookup,
// preventing a data race with a concurrent Rebuild that atomically swaps
// idx.store.
func (idx *Index) Lookup(id NodeID) *Node {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.store.Node(id)
}

// Store returns a snapshot pointer to the underlying Store at the instant of
// the call. The pointer is valid for READ-ONLY use only. It carries no
// durability guarantee: a concurrent Rebuild may atomically replace idx.store
// at any time after Store() returns, making the returned pointer stale.
// Callers must NOT retain the pointer across yield points and must NOT call
// AddNode / AddEdge through it; doing so introduces a data race. Use the
// Index query methods (Search, Callers, Callees, Impact, Lookup) for safe
// access.
func (idx *Index) Store() *Store {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.store
}

// parseAll walks root recursively, calling ParseDir on each subdirectory that
// contains at least one non-test .go file. The pkgPath for each subdirectory is
// derived by appending the relative path from root to idx.pkgPath.
func parseAll(root, basePkgPath string, store *Store) error {
	// Collect unique directories that contain .go files.
	dirs := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			dirs[filepath.Dir(path)] = true
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk %s: %w", root, err)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("abs %s: %w", root, err)
	}
	for dir := range dirs {
		absDir, aErr := filepath.Abs(dir)
		if aErr != nil {
			continue
		}
		rel, rErr := filepath.Rel(absRoot, absDir)
		if rErr != nil {
			continue
		}
		pkgPath := basePkgPath
		if rel != "." {
			pkgPath = basePkgPath + "/" + filepath.ToSlash(rel)
		}
		if pErr := ParseDir(dir, pkgPath, store); pErr != nil {
			// Non-fatal: a single bad package should not abort the whole walk.
			continue
		}
	}
	return nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
