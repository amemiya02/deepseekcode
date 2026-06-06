package codegraph

import "sort"

// RankByPageRank runs `iterations` steps of PageRank over the CALLS edge set
// and returns nodes sorted descending by score. A sensible value for
// iterations is 20–100 for small graphs; higher values cost more but
// converge more tightly. When iterations is 0 or negative the loop is
// skipped and every node receives an equal score of 1/n, with ties broken
// arbitrarily by the final sort.
func RankByPageRank(s *Store, iterations int) []*Node {
	allNodes := s.AllNodes()
	if len(allNodes) == 0 {
		return []*Node{}
	}
	const damping = 0.85
	n := len(allNodes)

	ids := make([]NodeID, 0, n)
	set := make(map[NodeID]struct{}, n)
	for _, node := range allNodes {
		ids = append(ids, node.ID)
		set[node.ID] = struct{}{}
	}

	score := make(map[NodeID]float64, n)
	for _, id := range ids {
		score[id] = 1.0 / float64(n)
	}

	for iter := 0; iter < iterations; iter++ {
		next := make(map[NodeID]float64, n)
		for _, id := range ids {
			next[id] = (1 - damping) / float64(n)
		}
		for _, id := range ids {
			outEdges := s.OutEdges(id)
			// Only count out-edges whose target is a registered node so the
			// denominator matches the mass actually distributed.
			var callOut int
			for _, e := range outEdges {
				if e.Kind == EdgeCalls {
					if _, ok := set[e.To]; ok {
						callOut++
					}
				}
			}
			if callOut == 0 {
				// dangling node: distribute evenly
				share := damping * score[id] / float64(n)
				for _, tid := range ids {
					next[tid] += share
				}
				continue
			}
			share := damping * score[id] / float64(callOut)
			for _, e := range outEdges {
				if e.Kind == EdgeCalls {
					if _, ok := set[e.To]; ok {
						next[e.To] += share
					}
				}
			}
		}
		score = next
	}

	// Reuse the allNodes slice directly to avoid a redundant Store lookup.
	sorted := make([]*Node, 0, n)
	for i := range allNodes {
		sorted = append(sorted, allNodes[i])
	}
	sort.Slice(sorted, func(i, j int) bool {
		return score[sorted[i].ID] > score[sorted[j].ID]
	})
	return sorted
}
