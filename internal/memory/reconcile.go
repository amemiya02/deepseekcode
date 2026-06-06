package memory

import (
	"math"
)

const reconcileThreshold = 0.82 // TF-cosine similarity threshold (plain term-frequency cosine, not BM25-weighted)

// findNearDuplicate scans existing records for one whose content is
// sufficiently similar to newContent (Mem0 update-in-place pattern).
// Must be called with s.mu held.
func (s *JSONLStore) findNearDuplicate(newContent string) *Memory {
	newToks := tokenize(newContent)
	best := -1.0
	var bestM *Memory
	for _, m := range s.records {
		sim := cosineSim(newToks, tokenize(m.Content))
		if sim > best {
			best = sim
			bestM = m
		}
	}
	if best >= reconcileThreshold {
		return bestM
	}
	return nil
}

func cosineSim(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	freq := func(toks []string) map[string]float64 {
		m := make(map[string]float64)
		for _, t := range toks {
			m[t]++
		}
		return m
	}
	fa, fb := freq(a), freq(b)
	dot, na, nb := 0.0, 0.0, 0.0
	for t, v := range fa {
		dot += v * fb[t]
		na += v * v
	}
	for _, v := range fb {
		nb += v * v
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func mergeTags(a, b []string) []string {
	seen := make(map[string]bool)
	for _, t := range a {
		seen[t] = true
	}
	out := append([]string{}, a...)
	for _, t := range b {
		if !seen[t] {
			out = append(out, t)
			seen[t] = true
		}
	}
	return out
}
