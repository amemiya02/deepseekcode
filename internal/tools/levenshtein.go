package tools

// LevenshteinLines computes the line-level Levenshtein distance between a and b.
// Each element is treated as an atomic token, so the cost is O(len(a)*len(b)).
func LevenshteinLines(a, b []string) int {
	n, m := len(a), len(b)
	if n == 0 {
		return m
	}
	if m == 0 {
		return n
	}

	// Two-row DP to keep memory O(min(n,m)).
	if m < n {
		a, b = b, a
		n, m = m, n
	}
	prev := make([]int, n+1)
	curr := make([]int, n+1)

	for i := 0; i <= n; i++ {
		prev[i] = i
	}

	for j := 1; j <= m; j++ {
		curr[0] = j
		for i := 1; i <= n; i++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[i] = min3(
				prev[i]+1,      // delete
				curr[i-1]+1,    // insert
				prev[i-1]+cost, // substitute
			)
		}
		prev, curr = curr, prev
	}
	return prev[n]
}

// ClosestMatch finds the element in haystack whose line-level distance to needle
// is smallest. Returns (-1, maxDist+1) if no element is within maxDist.
func ClosestMatch(needle string, haystack []string, maxDist int) (idx, dist int) {
	needleLines := splitLines(needle)
	bestIdx := -1
	bestDist := maxDist + 1
	for i, hay := range haystack {
		d := LevenshteinLines(needleLines, splitLines(hay))
		if d < bestDist {
			bestDist = d
			bestIdx = i
		}
	}
	return bestIdx, bestDist
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
