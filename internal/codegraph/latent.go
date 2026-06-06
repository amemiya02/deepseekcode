package codegraph

import (
	"fmt"
	"strings"
)

// defaultPageRankIters is the number of PageRank iterations used by
// NewLatentInjector. Twenty iterations is sufficient for convergence on
// typical codebase graphs (empirically within 1e-6 delta).
const defaultPageRankIters = 20

// LatentInjector renders the top-N most central symbols from an Index into a
// compact markdown snippet suitable for injection into the dynamic context
// window (i.e., AFTER prompt.DynamicContextBoundary).
type LatentInjector struct {
	idx   *Index
	topN  int
	iters int
}

// NewLatentInjector creates a LatentInjector that will render the top-N
// symbols by PageRank centrality.
func NewLatentInjector(idx *Index, topN int) *LatentInjector {
	return &LatentInjector{idx: idx, topN: topN, iters: defaultPageRankIters}
}

// escapePipe replaces bare pipe characters in a markdown table cell value so
// they do not corrupt the table structure.
func escapePipe(s string) string {
	return strings.ReplaceAll(s, "|", `\|`)
}

// Render returns a markdown-formatted symbol table of the most central nodes.
// The output is intended for placement AFTER prompt.DynamicContextBoundary.
func (li *LatentInjector) Render() string {
	// Short-circuit: nothing to render. A non-positive topN (including
	// negative values) would otherwise index ranked[:limit] with a negative
	// limit and panic, so guard <= 0.
	if li.topN <= 0 {
		return ""
	}

	store := li.idx.Store()
	ranked := RankByPageRank(store, li.iters)

	limit := li.topN
	if limit > len(ranked) {
		limit = len(ranked)
	}
	ranked = ranked[:limit]

	if len(ranked) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("<!-- codegraph:latent -->\n")
	sb.WriteString("| Symbol | Kind | File | Line |\n")
	sb.WriteString("|---|---|---|---|\n")
	for _, n := range ranked {
		sb.WriteString(fmt.Sprintf("| `%s` | %s | %s | %d |\n",
			escapePipe(n.Name),
			escapePipe(n.Kind.String()),
			escapePipe(n.File),
			n.Line))
	}
	sb.WriteString("<!-- /codegraph:latent -->\n")
	return sb.String()
}
