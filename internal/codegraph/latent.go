package codegraph

import (
	"fmt"
	"strings"
)

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
	return &LatentInjector{idx: idx, topN: topN, iters: 20}
}

// Render returns a markdown-formatted symbol table of the most central nodes.
// The output is intended for placement AFTER prompt.DynamicContextBoundary.
func (li *LatentInjector) Render() string {
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
		sb.WriteString(fmt.Sprintf("| `%s` | %s | %s | %d |\n", n.Name, n.Kind.String(), n.File, n.Line))
	}
	sb.WriteString("<!-- /codegraph:latent -->\n")
	return sb.String()
}
