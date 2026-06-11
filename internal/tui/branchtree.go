package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/session"
)

// renderBranchTree renders a deterministic ASCII tree of session branches.
// The active session is marked with "->". Siblings are sorted by ID for
// determinism (no wall-clock ordering).
func renderBranchTree(sessions []session.Session, activeID string) string {
	if len(sessions) == 0 {
		return "(no sessions)"
	}
	// Build parent->children map
	children := make(map[string][]session.Session)
	roots := make([]string, 0)
	byID := make(map[string]session.Session, len(sessions))
	for _, s := range sessions {
		byID[s.ID] = s
		if s.ParentID == "" {
			roots = append(roots, s.ID)
		} else {
			children[s.ParentID] = append(children[s.ParentID], s)
		}
	}
	sort.Strings(roots) // deterministic

	var sb strings.Builder
	var dfs func(id string, depth int)
	dfs = func(id string, depth int) {
		s := byID[id]
		indent := strings.Repeat("  ", depth)
		marker := "  "
		if id == activeID {
			marker = "-> "
		}
		title := s.Summary
		if title == "" {
			title = id
		}
		bp := ""
		if s.BranchPoint > 0 && s.ParentID != "" {
			bp = fmt.Sprintf(" (fork @ msg %d)", s.BranchPoint)
		}
		fmt.Fprintf(&sb, "%s%s%s%s\n", indent, marker, title, bp)

		kids := children[id]
		sort.Slice(kids, func(i, j int) bool { return kids[i].ID < kids[j].ID })
		for _, k := range kids {
			dfs(k.ID, depth+1)
		}
	}
	for _, id := range roots {
		dfs(id, 0)
	}
	return sb.String()
}
