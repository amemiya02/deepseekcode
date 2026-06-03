package codegraph

import "sort"

// RankByPageRank runs `iterations` steps of PageRank over the CALLS edge set
// and returns nodes sorted descending by score.
func RankByPageRank(s *Store, iterations int) []*Node {
	allNodes := s.AllNodes()
	if len(allNodes) == 0 {
		return []*Node{}
	}
	const damping = 0.85
	n := len(allNodes)

	ids := make([]NodeID, 0, n)
	for _, node := range allNodes {
		ids = append(ids, node.ID)
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
			var callOut int
			for _, e := range outEdges {
				if e.Kind == EdgeCalls {
					callOut++
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
					next[e.To] += share
				}
			}
		}
		score = next
	}

	sorted := make([]*Node, 0, n)
	for _, id := range ids {
		sorted = append(sorted, s.Node(id))
	}
	sort.Slice(sorted, func(i, j int) bool {
		return score[sorted[i].ID] > score[sorted[j].ID]
	})
	return sorted
}
