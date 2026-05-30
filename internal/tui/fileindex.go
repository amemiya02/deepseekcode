// fileindex.go backs the `@` file-mention menu. It is the file source for the
// completions popup: a depth-limited walk of
// the working directory producing repo-relative paths, skipping the usual noise
// dirs (.git, node_modules, vendor) and any dot-directory, capped so a giant
// tree can never blow up memory or the render path.
//
// The walk is intentionally off the UI hot path: App kicks it off in a tea.Cmd
// (eagerly at startup, see Init) and the result is delivered as a fileIndexMsg
// that App caches. Until that message lands the popup shows a single
// "(indexing files…)" placeholder row. Ranking is left to fuzzyMatch over the
// relative path (the same matcher the `/` menu and pickers share), so no
// path-aware scoring lives here.
package tui

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// fileIndexCap bounds the candidate set. ~2000 entries keeps the fuzzy filter
// and popup render cheap on huge monorepos; beyond it the walk stops early and
// returns what it has (the user narrows with the query anyway).
const fileIndexCap = 2000

// fileIndexMaxDepth limits how deep the walk descends below the root. The `@`
// menu is for naming files the user is already thinking about, not exhaustively
// mirroring the tree, so a shallow cap both bounds cost and keeps the candidate
// list legible.
const fileIndexMaxDepth = 8

// fileIndexMsg delivers the completed file walk back to Update off the UI
// goroutine. paths are repo-relative (slash-separated), sorted, and capped at
// fileIndexCap. An empty slice is a valid result (an empty or unreadable cwd).
type fileIndexMsg struct{ paths []string }

// indexFilesCmd returns a tea.Cmd that walks root in the background and delivers
// the result as a fileIndexMsg. It never blocks the UI loop: the walk runs
// inside the Cmd closure that tea schedules off the render goroutine. A blank
// root yields an empty index rather than walking the process cwd.
func indexFilesCmd(root string) tea.Cmd {
	return func() tea.Msg {
		return fileIndexMsg{paths: walkFiles(root, fileIndexCap)}
	}
}

// walkFiles returns up to limit repo-relative file paths under root, skipping
// .git, node_modules, vendor, and any dot-directory, and never descending past
// fileIndexMaxDepth. Paths use forward slashes regardless of OS so they read
// the same as the agent's tool paths and match cleanly against a typed query.
// The result is sorted for a stable, deterministic candidate order (fuzzyMatch
// then re-ranks by score). A missing or unreadable root yields nil.
func walkFiles(root string, limit int) []string {
	if root == "" || limit <= 0 {
		return nil
	}
	out := make([]string, 0, 256)
	// filepath.WalkDir visits in lexical order; we stop early once the cap is
	// hit by returning a sentinel error from the walk func.
	stop := fs.SkipAll
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Unreadable entry (permissions, race): skip the subtree if it's a
			// dir, otherwise skip the single entry. Never abort the whole walk.
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if path == root {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		if d.IsDir() {
			name := d.Name()
			if skipDirName(name) {
				return fs.SkipDir
			}
			// Depth = number of path segments; the root's direct children are
			// depth 1. Stop descending past the cap without dropping siblings.
			if strings.Count(rel, "/")+1 > fileIndexMaxDepth {
				return fs.SkipDir
			}
			return nil
		}
		out = append(out, rel)
		if len(out) >= limit {
			return stop
		}
		return nil
	})
	sort.Strings(out)
	return out
}

// skipDirName reports whether a directory should be pruned from the walk: the
// VCS/dependency noise dirs and any hidden dot-directory (".git", ".idea", …).
// "." and ".." never appear as DirEntry names from WalkDir, but the leading-dot
// rule covers every other hidden dir uniformly.
func skipDirName(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor":
		return true
	}
	return strings.HasPrefix(name, ".")
}

// fileCompletionItems maps cached repo-relative paths into popup rows for the
// `@` menu: the relative path is both the inserted text and the label, with no
// detail column (the path is self-describing). SetQuery later fuzzy-filters and
// ranks these against the text typed after the '@'.
func fileCompletionItems(paths []string) []complItem {
	items := make([]complItem, 0, len(paths))
	for _, p := range paths {
		items = append(items, complItem{insert: p, label: p, kind: fileCmd})
	}
	return items
}

// fileIndexingItems is the single placeholder row shown while the background
// walk is still in flight. Its empty insert makes acceptCompletion a no-op
// close (the user can't accept a path that isn't loaded yet), and the
// non-matching '\x00' query sentinel is unused — SetQuery filters by label, and
// this label deliberately won't survive a real query so the placeholder hides
// the moment the user starts typing a path fragment that doesn't match it.
func fileIndexingItems() []complItem {
	return []complItem{{insert: "", label: "(indexing files…)", kind: fileCmd}}
}

// readableDir reports whether p exists and is a directory we can open. Used to
// keep the eager startup Cmd from spinning on a bogus cwd; not strictly needed
// for correctness (walkFiles tolerates a bad root) but it avoids scheduling a
// pointless walk.
func readableDir(p string) bool {
	if p == "" {
		return false
	}
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
